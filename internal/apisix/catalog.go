package apisix

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Schemas of the resources the UI builds forms for (GET /apisix/admin/schema/<name>).
var schemaResources = []string{
	"route", "service", "upstream", "consumer", "consumer_group", "ssl",
	"global_rule", "plugin_config", "stream_route", "proto", "credential",
}

const catalogTTL = 10 * time.Minute

// Catalog caches the plugin and resource schemas published by APISIX. Besides
// feeding the UI forms, it provides each plugin's encrypt_fields, which APISIX
// itself declares as sensitive and the dashboard therefore masks.
type Catalog struct {
	client *Client

	mu     sync.RWMutex
	data   json.RawMessage
	fields map[string][][]string
	loaded time.Time
}

func NewCatalog(c *Client) *Catalog {
	return &Catalog{client: c, fields: map[string][][]string{}}
}

type pluginInfo struct {
	Schema         map[string]any `json:"schema"`
	ConsumerSchema map[string]any `json:"consumer_schema"`
	MetadataSchema map[string]any `json:"metadata_schema"`
}

func (c *Catalog) get(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	resp, err := c.client.Do(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	if !resp.OK() {
		return nil, fmt.Errorf("GET %s: HTTP %d", path, resp.Status)
	}
	return resp.Body, nil
}

// Refresh reloads every schema from the Admin API.
func (c *Catalog) Refresh(ctx context.Context) error {
	httpPlugins, err := c.get(ctx, "plugins", url.Values{"all": {"true"}, "subsystem": {"http"}})
	if err != nil {
		return err
	}
	// Stream mode may be disabled; that is not an error for the catalog.
	streamPlugins, err := c.get(ctx, "plugins", url.Values{"all": {"true"}, "subsystem": {"stream"}})
	if err != nil {
		streamPlugins = json.RawMessage("{}")
	}
	resources := map[string]json.RawMessage{}
	for _, r := range schemaResources {
		if s, err := c.get(ctx, "schema/"+r, nil); err == nil {
			resources[r] = s
		}
	}

	fields := map[string][][]string{}
	for _, raw := range []json.RawMessage{httpPlugins, streamPlugins} {
		var plugins map[string]pluginInfo
		if err := json.Unmarshal(raw, &plugins); err != nil {
			return fmt.Errorf("decode plugins: %w", err)
		}
		for name, p := range plugins {
			for _, s := range []map[string]any{p.Schema, p.ConsumerSchema, p.MetadataSchema} {
				for _, f := range encryptFields(s) {
					fields[name] = appendUnique(fields[name], strings.Split(f, "."))
				}
			}
		}
	}

	data, err := json.Marshal(map[string]any{
		"plugins":        httpPlugins,
		"stream_plugins": streamPlugins,
		"resources":      resources,
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.data, c.fields, c.loaded = data, fields, time.Now()
	c.mu.Unlock()
	return nil
}

func encryptFields(schema map[string]any) []string {
	list, _ := schema["encrypt_fields"].([]any)
	out := make([]string, 0, len(list))
	for _, f := range list {
		if s, ok := f.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appendUnique(paths [][]string, p []string) [][]string {
	for _, q := range paths {
		if strings.Join(q, ".") == strings.Join(p, ".") {
			return paths
		}
	}
	return append(paths, p)
}

// JSON returns the cached catalog, refreshing it when stale. A stale copy is
// served if APISIX is momentarily unreachable.
func (c *Catalog) JSON(ctx context.Context) (json.RawMessage, error) {
	c.mu.RLock()
	data, loaded := c.data, c.loaded
	c.mu.RUnlock()
	if data != nil && time.Since(loaded) < catalogTTL {
		return data, nil
	}
	if err := c.Refresh(ctx); err != nil {
		if data != nil {
			slog.Warn("schema catalog refresh failed; serving cached copy", "err", err)
			return data, nil
		}
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data, nil
}

// EncryptFields returns the sensitive field paths of a plugin.
func (c *Catalog) EncryptFields(plugin string) [][]string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fields[plugin]
}

// Run loads the catalog at startup (retrying until APISIX answers) and keeps
// it fresh so masking always knows the current encrypt_fields.
func (c *Catalog) Run(ctx context.Context) {
	go func() {
		delay := 2 * time.Second
		for {
			err := c.Refresh(ctx)
			if err == nil {
				slog.Info("schema catalog loaded")
				delay = catalogTTL
			} else {
				slog.Warn("schema catalog load failed", "err", err)
				delay = min(delay*2, time.Minute)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}()
}
