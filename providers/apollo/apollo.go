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
func (p *Provider) Watch() (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 1)

	go func() {
		defer close(ch)
		ticker := time.NewTicker(time.Duration(p.cfg.SyncInterval) * time.Second)
		if p.cfg.SyncInterval <= 0 {
			ticker = time.NewTicker(60 * time.Second)
		}
		defer ticker.Stop()

		for range ticker.C {
			data, err := p.Fetch()
			if err != nil {
				log.Printf("[apollo] watch fetch error: %v", err)
				continue
			}

			// Check if actually changed
			p.mu.Lock()
			cached := p.cache
			p.cache = data
			nid := p.notificationID
			if nid2, ok := getIntFromMap(data, "notificationId"); ok {
				p.notificationID = nid2
			}
			p.mu.Unlock()

			if nid == p.notificationID {
				continue // No change
			}

			// Extract configurations
			cfgs, ok := data["configurations"].(map[string]any)
			if !ok {
				continue
			}

			select {
			case ch <- cfgs:
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
