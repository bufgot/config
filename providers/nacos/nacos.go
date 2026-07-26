// Package nacos provides a RemoteProvider implementation for Nacos config center.
//
// Usage:
//
//	rc := config.ParseRemoteConfig(app.Viper(), "nacos")
//	if rc != nil {
//	    provider, err := nacos.New(rc)
//	    if err != nil { ... }
//	    app.UseRemote(provider)
//	}
package nacos

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bufgot/config"
)

// Provider implements config.RemoteProvider for Nacos config center.
// Communication is via Nacos HTTP Open API.
type Provider struct {
	cfg        *config.RemoteConfig
	httpClient *http.Client
	mu         sync.Mutex

	// lastHash caches the MD5 hash of the last fetched config content,
	// used for change detection in polling mode.
	lastHash string
}

// New creates a Nacos Provider.
//
// Required RemoteConfig fields:
//   - Endpoints[0]: Nacos server address (e.g. "http://127.0.0.1:8848")
//   - Namespace: Nacos tenant/namespace ID (optional, empty for public namespace)
//
// Extra fields:
//   - "dataId": config data ID (defaults to AppID if not set in Extra)
//   - "group": config group name (defaults to "DEFAULT_GROUP")
func New(rc *config.RemoteConfig) (*Provider, error) {
	if rc == nil || len(rc.Endpoints) == 0 {
		return nil, fmt.Errorf("nacos: missing endpoints")
	}
	if dataID(rc) == "" {
		return nil, fmt.Errorf("nacos: missing dataId (set app_id or extra.dataId)")
	}

	interval := rc.SyncInterval
	if interval <= 0 {
		interval = 30 // default 30 seconds
	}

	log.Printf("[nacos] provider created: addr=%s dataId=%s group=%s ns=%s interval=%ds",
		rc.Endpoints[0], dataID(rc), group(rc), rc.Namespace, interval)

	return &Provider{
		cfg: rc,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string { return "nacos" }

// Fetch pulls full config from Nacos via GET /v1/cs/configs.
func (p *Provider) Fetch() (map[string]any, error) {
	addr := p.cfg.Endpoints[0]
	reqURL := fmt.Sprintf("%s/v1/cs/configs", addr)

	params := url.Values{}
	params.Set("dataId", dataID(p.cfg))
	params.Set("group", group(p.cfg))
	if p.cfg.Namespace != "" {
		params.Set("tenant", p.cfg.Namespace)
	}

	fullURL := reqURL + "?" + params.Encode()
	log.Printf("[nacos] fetching config from: %s", fullURL)

	resp, err := p.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("nacos: get %s: %w", fullURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nacos: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nacos: status %d body=%s", resp.StatusCode, string(body))
	}

	// Nacos returns raw config content; try JSON first, fall back to wrap.
	result, contentHash := parseNacosResponse(body)

	p.mu.Lock()
	p.lastHash = contentHash
	p.mu.Unlock()

	return result, nil
}

// Watch starts periodic polling and returns a config change channel.
// Uses MD5 hash comparison to detect content changes.
func (p *Provider) Watch(ctx context.Context) (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 1)

	go func() {
		defer close(ch)

		interval := p.cfg.SyncInterval
		if interval <= 0 {
			interval = 30
		}
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			data, hash, err := p.fetchAndHash()
			if err != nil {
				log.Printf("[nacos] watch fetch error: %v", err)
				continue
			}

			p.mu.Lock()
			changed := p.lastHash != hash
			if changed {
				p.lastHash = hash
			}
			p.mu.Unlock()

			if !changed {
				continue
			}

			select {
			case ch <- data:
				log.Println("[nacos] config change pushed to channel")
			default:
				log.Println("[nacos] channel full, dropping change")
			}
		}
	}()

	return ch, nil
}

// fetchAndHash fetches config and returns parsed data + MD5 hash.
func (p *Provider) fetchAndHash() (map[string]any, string, error) {
	addr := p.cfg.Endpoints[0]
	reqURL := fmt.Sprintf("%s/v1/cs/configs", addr)

	params := url.Values{}
	params.Set("dataId", dataID(p.cfg))
	params.Set("group", group(p.cfg))
	if p.cfg.Namespace != "" {
		params.Set("tenant", p.cfg.Namespace)
	}

	fullURL := reqURL + "?" + params.Encode()

	resp, err := p.httpClient.Get(fullURL)
	if err != nil {
		return nil, "", fmt.Errorf("nacos: get %s: %w", fullURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("nacos: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("nacos: status %d body=%s", resp.StatusCode, string(body))
	}

	result, hash := parseNacosResponse(body)
	return result, hash, nil
}

// parseNacosResponse parses raw Nacos config content.
// Tries JSON unmarshal first; if that fails, wraps the raw content under key "content".
// Returns the parsed map and the MD5 hash of the raw body for change detection.
func parseNacosResponse(body []byte) (map[string]any, string) {
	hash := fmt.Sprintf("%x", md5.Sum(body))

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		result = map[string]any{"content": string(body)}
	}
	return result, hash
}

// dataID extracts the dataId from RemoteConfig.
// Priority: Extra["dataId"] > AppID.
func dataID(rc *config.RemoteConfig) string {
	if rc.Extra != nil {
		if v, ok := rc.Extra["dataId"].(string); ok && v != "" {
			return v
		}
	}
	return rc.AppID
}

// group extracts the group from RemoteConfig.
// Priority: Extra["group"] > "DEFAULT_GROUP".
func group(rc *config.RemoteConfig) string {
	if rc.Extra != nil {
		if v, ok := rc.Extra["group"].(string); ok && v != "" {
			return v
		}
	}
	return "DEFAULT_GROUP"
}

// Ensure interface implementation
var _ config.RemoteProvider = (*Provider)(nil)
