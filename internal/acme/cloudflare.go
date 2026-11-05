package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
)

// Cloudflare manages the _acme-challenge TXT records through the Cloudflare
// API. The token only needs Zone:DNS:Edit (and Zone:Read) on the zones used.
type Cloudflare struct {
	base  string
	token string
	hc    *http.Client

	mu    sync.Mutex
	zones map[string]Zone // fqdn → zone
}

type Zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

func NewCloudflare(baseURL, token string, hc *http.Client) *Cloudflare {
	return &Cloudflare{base: strings.TrimRight(baseURL, "/"), token: token, hc: hc, zones: map[string]Zone{}}
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

func (c *Cloudflare) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("cloudflare: %w", err)
	}
	var env cfEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("cloudflare: HTTP %d with an invalid response", resp.StatusCode)
	}
	if !env.Success || resp.StatusCode >= 300 {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%s (%d)", e.Message, e.Code))
		}
		if len(msgs) == 0 {
			msgs = append(msgs, http.StatusText(resp.StatusCode))
		}
		return fmt.Errorf("cloudflare: HTTP %d: %s", resp.StatusCode, strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// Verify checks the token is active.
func (c *Cloudflare) Verify(ctx context.Context) error {
	var r struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, http.MethodGet, "/user/tokens/verify", nil, nil, &r); err != nil {
		return err
	}
	if r.Status != "active" {
		return fmt.Errorf("cloudflare: token status %q", r.Status)
	}
	return nil
}

// Zones lists the zones the token can see.
func (c *Cloudflare) Zones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	err := c.call(ctx, http.MethodGet, "/zones", url.Values{"per_page": {"50"}}, nil, &zones)
	return zones, err
}

// ZoneFor finds the zone that holds fqdn, trying each parent domain.
func (c *Cloudflare) ZoneFor(ctx context.Context, fqdn string) (Zone, error) {
	fqdn = strings.TrimSuffix(strings.ToLower(fqdn), ".")
	c.mu.Lock()
	z, ok := c.zones[fqdn]
	c.mu.Unlock()
	if ok {
		return z, nil
	}
	labels := strings.Split(fqdn, ".")
	for i := 0; i < len(labels)-1; i++ {
		name := strings.Join(labels[i:], ".")
		var zones []Zone
		if err := c.call(ctx, http.MethodGet, "/zones", url.Values{"name": {name}}, nil, &zones); err != nil {
			return Zone{}, err
		}
		if len(zones) > 0 {
			c.mu.Lock()
			c.zones[fqdn] = zones[0]
			c.mu.Unlock()
			return zones[0], nil
		}
	}
	return Zone{}, i18n.E("acme.no_zone", "no Cloudflare zone reachable with the token contains {fqdn}", "fqdn", fqdn)
}

// CreateTXT creates a TXT record and returns a reference for DeleteTXT.
func (c *Cloudflare) CreateTXT(ctx context.Context, fqdn, value string) (string, error) {
	zone, err := c.ZoneFor(ctx, fqdn)
	if err != nil {
		return "", err
	}
	var rec struct {
		ID string `json:"id"`
	}
	body := map[string]any{
		"type": "TXT", "name": strings.TrimSuffix(fqdn, "."), "content": value, "ttl": 60,
		"comment": "apisix-dashboard: temporary ACME challenge",
	}
	if err := c.call(ctx, http.MethodPost, "/zones/"+url.PathEscape(zone.ID)+"/dns_records", nil, body, &rec); err != nil {
		return "", err
	}
	if rec.ID == "" {
		return "", errors.New("cloudflare: record created without an id")
	}
	return zone.ID + "/" + rec.ID, nil
}

func (c *Cloudflare) DeleteTXT(ctx context.Context, ref string) error {
	zoneID, recID, ok := strings.Cut(ref, "/")
	if !ok {
		return fmt.Errorf("cloudflare: invalid reference %q", ref)
	}
	return c.call(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recID), nil, nil, nil)
}
