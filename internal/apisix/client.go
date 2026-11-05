// Package apisix talks to the APISIX Admin API. The admin key only lives in
// this process: browsers never see it.
package apisix

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseSize = 32 << 20

type Client struct {
	base string
	key  string
	hc   *http.Client
}

func NewClient(baseURL, key, caFile string, timeout time.Duration) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile) // #nosec G304 -- CA path comes from operator config
		if err != nil {
			return nil, fmt.Errorf("admin api CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("admin api CA: no certificates found")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		key:  key,
		hc:   &http.Client{Timeout: timeout, Transport: tr},
	}, nil
}

type Response struct {
	Status int
	Server string
	Body   []byte
}

func (r *Response) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Do calls /apisix/admin/<path>. path segments must already be validated.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body []byte) (*Response, error) {
	u := c.base + "/apisix/admin/" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("X-API-KEY", c.key)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Server: resp.Header.Get("Server"), Body: b}, nil
}
