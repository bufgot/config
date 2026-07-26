// Package etcd provides a RemoteProvider implementation for Etcd config center.
//
// Usage:
//
//	rc := config.ParseRemoteConfig(app.Viper(), "etcd")
//	if rc != nil {
//	    provider, err := etcd.New(rc)
//	    if err != nil { ... }
//	    app.UseRemote(provider)
//	}
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/bufgot/config"
)

// Provider implements config.RemoteProvider for Etcd config center.
type Provider struct {
	cfg    *config.RemoteConfig
	client *clientv3.Client
	mu     sync.Mutex
}

// New creates an Etcd Provider.
func New(rc *config.RemoteConfig) (*Provider, error) {
	if rc == nil || len(rc.Endpoints) == 0 {
		return nil, fmt.Errorf("etcd: missing endpoints")
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   rc.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd: connect %v: %w", rc.Endpoints, err)
	}

	log.Printf("[etcd] provider created: endpoints=%v ns=%s", rc.Endpoints, rc.Namespace)
	return &Provider{
		cfg:    rc,
		client: cli,
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string { return "etcd" }

// Fetch pulls full config from Etcd (prefix scan).
func (p *Provider) Fetch() (map[string]any, error) {
	prefix := p.keyPrefix()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := p.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd: get prefix %s: %w", prefix, err)
	}

	result := make(map[string]any)
	for _, kv := range resp.Kvs {
		key := strings.TrimPrefix(string(kv.Key), prefix)
		key = strings.TrimLeft(key, "/")

		// Try JSON parsing, fall back to string
		var val any
		if err := json.Unmarshal(kv.Value, &val); err != nil {
			val = string(kv.Value)
		}
		result[key] = val
	}

	return result, nil
}

// Watch monitors changes under the Etcd prefix.
func (p *Provider) Watch(ctx context.Context) (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 1)
	prefix := p.keyPrefix()

	go func() {
		defer close(ch)
		defer p.client.Close()

		watchChan := p.client.Watch(ctx, prefix, clientv3.WithPrefix())

		for wr := range watchChan {
			if wr.Err() != nil {
				log.Printf("[etcd] watch error: %v", wr.Err())
				return
			}

			// Re-fetch full config
			data, err := p.Fetch()
			if err != nil {
				log.Printf("[etcd] fetch on watch event: %v", err)
				continue
			}

			select {
			case ch <- data:
				log.Println("[etcd] config change pushed to channel")
			default:
				log.Println("[etcd] channel full, dropping change")
			}
		}
	}()

	return ch, nil
}

// keyPrefix returns the Etcd key prefix.
func (p *Provider) keyPrefix() string {
	if p.cfg.Namespace != "" {
		return fmt.Sprintf("/config/%s/", p.cfg.Namespace)
	}
	if p.cfg.AppID != "" {
		return fmt.Sprintf("/config/%s/", p.cfg.AppID)
	}
	return "/config/"
}

// Ensure interface implementation
var _ config.RemoteProvider = (*Provider)(nil)
