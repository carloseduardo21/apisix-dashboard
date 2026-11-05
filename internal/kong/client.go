// Package kong reads a Kong Gateway (3.x, database mode) through its Admin API
// and translates its configuration into APISIX objects, so it can be migrated
// in steps: one service, its routes, upstream and plugins at a time.
//
// The dashboard only reads from Kong. Credentials fetched from it (API keys,
// JWT and HMAC secrets) stay in memory and are sent only to APISIX.
package kong

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxResponseSize = 32 << 20
	pageSize        = "1000"
)

type Client struct {
	base        string
	tokenHeader string
	token       string
	hc          *http.Client
}

// NewClient builds a read-only Admin API client. workspace is only used by
// Kong Enterprise; token is sent in tokenHeader (Kong-Admin-Token by default).
func NewClient(baseURL, workspace, tokenHeader, token, caFile string, timeout time.Duration) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("kong admin api CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("kong admin api CA: no certificates found")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	base := strings.TrimRight(baseURL, "/")
	if workspace != "" {
		base += "/" + url.PathEscape(workspace)
	}
	if tokenHeader == "" {
		tokenHeader = "Kong-Admin-Token"
	}
	return &Client{base: base, tokenHeader: tokenHeader, token: token, hc: &http.Client{Timeout: timeout, Transport: tr}}, nil
}

// URL is the Admin API address, shown in the UI.
func (c *Client) URL() string { return c.base }

var errNotFound = errors.New("not found")

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set(c.tokenHeader, c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("GET %s: HTTP %d (verifique o token da Admin API do Kong)", path, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.Unmarshal(b, v)
}

// list reads every page of a collection. A collection that does not exist
// (e.g. /hmac-auths when the plugin is not loaded) is empty.
func list[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	out := []T{}
	q := url.Values{"size": {pageSize}}
	for {
		var page struct {
			Data   []T    `json:"data"`
			Offset string `json:"offset"`
		}
		err := c.get(ctx, path, q, &page)
		if errors.Is(err, errNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		if page.Offset == "" {
			return out, nil
		}
		q.Set("offset", page.Offset)
	}
}

// Info identifies the Kong node.
type Info struct {
	Version  string `json:"version"`
	Hostname string `json:"hostname"`
	Database string `json:"database"`
}

func (c *Client) Info(ctx context.Context) (*Info, error) {
	var root struct {
		Version       string `json:"version"`
		Hostname      string `json:"hostname"`
		Configuration struct {
			Database string `json:"database"`
		} `json:"configuration"`
	}
	if err := c.get(ctx, "/", nil, &root); err != nil {
		return nil, err
	}
	return &Info{Version: root.Version, Hostname: root.Hostname, Database: root.Configuration.Database}, nil
}

// Snapshot reads everything the migration needs.
func (c *Client) Snapshot(ctx context.Context) (*Snapshot, error) {
	s := &Snapshot{Targets: map[string][]Target{}}
	var err error
	if s.Services, err = list[Service](ctx, c, "/services"); err != nil {
		return nil, err
	}
	if s.Routes, err = list[Route](ctx, c, "/routes"); err != nil {
		return nil, err
	}
	if s.Upstreams, err = list[Upstream](ctx, c, "/upstreams"); err != nil {
		return nil, err
	}
	for _, u := range s.Upstreams {
		targets, err := list[Target](ctx, c, "/upstreams/"+url.PathEscape(u.ID)+"/targets")
		if err != nil {
			return nil, err
		}
		s.Targets[u.ID] = targets
	}
	if s.Plugins, err = list[Plugin](ctx, c, "/plugins"); err != nil {
		return nil, err
	}
	if s.Consumers, err = list[Consumer](ctx, c, "/consumers"); err != nil {
		return nil, err
	}
	if s.KeyAuths, err = list[KeyAuth](ctx, c, "/key-auths"); err != nil {
		return nil, err
	}
	if s.BasicAuths, err = list[BasicAuth](ctx, c, "/basic-auths"); err != nil {
		return nil, err
	}
	if s.JWTs, err = list[JWT](ctx, c, "/jwts"); err != nil {
		return nil, err
	}
	if s.HMACAuths, err = list[HMACAuth](ctx, c, "/hmac-auths"); err != nil {
		return nil, err
	}
	if s.ACLs, err = list[ACL](ctx, c, "/acls"); err != nil {
		return nil, err
	}
	s.index()
	return s, nil
}
