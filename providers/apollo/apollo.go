// Package apollo provides a RemoteProvider implementation for Apollo config center.
//
// Usage:
//
//	rc := config.ParseRemoteConfig(app.Viper(), "apollo")
//	if rc != nil {
//	    provider, err := apollo.New(rc)
//	    if err != nil { ... }
//	    app.UseRemote(provider)
//	}
package apollo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bufgot/config"
)

// Provider implements config.RemoteProvider for Apollo config center.
type Provider struct {
	cfg        *config.RemoteConfig
	httpClient *http.Client
	mu         sync.Mutex

	// notificationID is used for Apollo long-polling change notifications
	notificationID int

	// Cached config to avoid channel write blocking
	cache map[string]any
}

// New creates an Apollo Provider.
func New(rc *config.RemoteConfig) (*Provider, error) {
	if rc == nil || len(rc.Endpoints) == 0 {
		return nil, fmt.Errorf("apollo: missing endpoints")
	}
	if rc.AppID == "" {
		return nil, fmt.Errorf("apollo: missing app_id")
	}

	interval := rc.SyncInterval
	if interval <= 0 {
		interval = 60 // default 60 seconds
	}

	log.Printf("[apollo] provider created: app=%s cluster=%s ns=%s endpoints=%v interval=%ds",
		rc.AppID, rc.Cluster, rc.Namespace, rc.Endpoints, interval)

	return &Provider{
		cfg: rc,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string { return "apollo" }

// Fetch pulls full config from Apollo.
func (p *Provider) Fetch() (map[string]any, error) {
	ns := p.namespace()
	url := fmt.Sprintf("%s/configs/%s/%s/%s",
		p.cfg.Endpoints[0], p.cfg.AppID, p.cfg.Cluster, ns)

	log.Printf("[apollo] fetching config from: %s", url)

	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("apollo: get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("apollo: status %d body=%s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("apollo: decode response: %w", err)
	}

	p.mu.Lock()
	p.cache = result
	// Extract notificationId
	if nid, ok := getIntFromMap(result, "notificationId"); ok {
		p.notificationID = nid
	}
	p.mu.Unlock()

	// Apollo response format: {"appId":..., "cluster":..., "namespaceName":..., "configurations":{...}}
	if cfgs, ok := result["configurations"].(map[string]any); ok {
		return cfgs, nil
	}

	return result, nil
}

// Watch starts Apollo polling and returns a config change channel.
func (p *Provider) Watch(ctx context.Context) (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 1)

	go func() {
		defer close(ch)

		interval := p.cfg.SyncInterval
		if interval <= 0 {
			interval = 60
		}
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			// Save old notificationID before Fetch (Fetch() updates it internally from raw API response).
			p.mu.Lock()
			oldNID := p.notificationID
			p.mu.Unlock()

			data, err := p.Fetch()
			if err != nil {
				log.Printf("[apollo] watch fetch error: %v", err)
				continue
			}

			// Compare updated notificationID.
			p.mu.Lock()
			changed := p.notificationID != oldNID
			p.mu.Unlock()

			if !changed {
				continue
			}

			select {
			case ch <- data:
				log.Println("[apollo] config change pushed to channel")
			default:
				log.Println("[apollo] channel full, dropping change")
			}
		}
	}()

	return ch, nil
}

// namespace returns the namespace, defaulting to "application".
func (p *Provider) namespace() string {
	if p.cfg.Namespace != "" {
		return p.cfg.Namespace
	}
	return "application"
}

// getIntFromMap extracts an int value from a map (compatible with float64 JSON parsing).
func getIntFromMap(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

// Ensure interface implementation
var _ config.RemoteProvider = (*Provider)(nil)
