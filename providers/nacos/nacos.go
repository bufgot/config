// Package nacos provides a RemoteProvider implementation for Nacos config center
// via gRPC using the official nacos-sdk-go.
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
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/bufgot/config"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Provider implements config.RemoteProvider for Nacos config center.
// Communication is via gRPC using nacos-sdk-go.
type Provider struct {
	cfg    *config.RemoteConfig
	client config_client.IConfigClient

	mu          sync.Mutex
	lastContent string
	listening   bool
}

// New creates a Nacos Provider via gRPC.
//
// Endpoints[0] must be "host:port" (e.g. "192.168.1.2:2848").
// Set Extra["grpcPort"] if the gRPC port differs from the default (port+1000).
//
// Extra fields:
//   - "dataId": config data ID (defaults to AppID if not set)
//   - "group": config group name (defaults to "DEFAULT_GROUP")
//   - "username": Nacos username (required if auth enabled)
//   - "password": Nacos password (required if auth enabled)
//   - "grpcPort": gRPC port override (int, e.g. 2849)
func New(rc *config.RemoteConfig) (*Provider, error) {
	if rc == nil || len(rc.Endpoints) == 0 {
		return nil, fmt.Errorf("nacos: missing endpoints")
	}
	if dataID(rc) == "" {
		return nil, fmt.Errorf("nacos: missing dataId (set app_id or extra.dataId)")
	}

	host, port, err := parseAddr(rc.Endpoints[0])
	if err != nil {
		return nil, fmt.Errorf("nacos: invalid endpoint %q: %w", rc.Endpoints[0], err)
	}

	username := extraStr(rc, "username")
	password := extraStr(rc, "password")

	cc := constant.ClientConfig{
		NamespaceId:         rc.Namespace,
		TimeoutMs:           10000,
		NotLoadCacheAtStart: true,
		Username:            username,
		Password:            password,
		LogLevel:            "warn",
	}

	grpcPort := port + 1000
	if v := extraInt(rc, "grpcPort"); v > 0 {
		grpcPort = v
	}

	sc := []constant.ServerConfig{{
		IpAddr:   host,
		Port:     uint64(port),
		GrpcPort: uint64(grpcPort),
	}}

	client, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  &cc,
		ServerConfigs: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: create client: %w", err)
	}

	log.Printf("[nacos] provider created: addr=%s:%d grpc=%d dataId=%s group=%s ns=%s",
		host, port, grpcPort, dataID(rc), group(rc), rc.Namespace)

	return &Provider{
		cfg:    rc,
		client: client,
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string { return "nacos" }

// Fetch pulls full config from Nacos via gRPC GetConfig.
func (p *Provider) Fetch() (map[string]any, error) {
	content, err := p.client.GetConfig(vo.ConfigParam{
		DataId: dataID(p.cfg),
		Group:  group(p.cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: get config: %w", err)
	}

	result := parseContent(content)

	p.mu.Lock()
	p.lastContent = content
	p.mu.Unlock()

	return result, nil
}

// Watch registers a Nacos ListenConfig callback and returns a change channel.
// On context cancellation, it calls CancelListenConfig to clean up.
func (p *Provider) Watch(ctx context.Context) (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 1)

	dataID := dataID(p.cfg)
	g := group(p.cfg)

	p.mu.Lock()
	if p.listening {
		p.mu.Unlock()
		return nil, fmt.Errorf("nacos: Watch already called")
	}
	p.listening = true
	p.mu.Unlock()

	err := p.client.ListenConfig(vo.ConfigParam{
		DataId: dataID,
		Group:  g,
		OnChange: func(namespace, group, dataId, data string) {
			result := parseContent(data)
			// Recover in case the channel is already closed
			// (CancelListenConfig may not prevent in-flight callbacks).
			func() {
				defer func() { recover() }()
				select {
				case ch <- result:
				default:
					log.Println("[nacos] config change channel full, dropping")
				}
			}()
		},
	})
	if err != nil {
		p.mu.Lock()
		p.listening = false
		p.mu.Unlock()
		return nil, fmt.Errorf("nacos: listen config: %w", err)
	}

	// Cancel listening when context is done
	go func() {
		<-ctx.Done()
		p.client.CancelListenConfig(vo.ConfigParam{
			DataId: dataID,
			Group:  g,
		})
		p.mu.Lock()
		p.listening = false
		p.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// parseContent parses raw Nacos config content.
// Tries JSON unmarshal first; if that fails, wraps under key "content".
func parseContent(content string) map[string]any {
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		result = map[string]any{"content": content}
	}
	return result
}

func dataID(rc *config.RemoteConfig) string {
	if rc.Extra != nil {
		if v, ok := rc.Extra["dataId"].(string); ok && v != "" {
			return v
		}
	}
	return rc.AppID
}

func group(rc *config.RemoteConfig) string {
	if rc.Extra != nil {
		if v, ok := rc.Extra["group"].(string); ok && v != "" {
			return v
		}
	}
	return "DEFAULT_GROUP"
}

// extraStr tries camelCase and snake_case keys from Extra.
func extraStr(rc *config.RemoteConfig, key string) string {
	if rc.Extra == nil {
		return ""
	}
	for _, k := range keyVariants(key) {
		if v, ok := rc.Extra[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// extraInt tries camelCase and snake_case keys from Extra.
func extraInt(rc *config.RemoteConfig, key string) int {
	if rc.Extra == nil {
		return 0
	}
	for _, k := range keyVariants(key) {
		switch v := rc.Extra[k].(type) {
		case int:
			if v != 0 {
				return v
			}
		case float64:
			if v != 0 {
				return int(v)
			}
		}
	}
	return 0
}

// keyVariants returns common case variants of a camelCase key.
func keyVariants(camel string) []string {
	// Build snake_case variant: insert _ before uppercase letters and lowercase.
	var snake []byte
	for i, c := range camel {
		if i > 0 && c >= 'A' && c <= 'Z' {
			snake = append(snake, '_')
		}
		snake = append(snake, byte(c)|0x20) // lowercase
	}
	return []string{camel, string(snake)}
}

// parseAddr splits "host:port" or "http://host:port" into (host, port, error).
func parseAddr(addr string) (string, int, error) {
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("split host:port: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}

	return host, port, nil
}

// Ensure interface implementation
var _ config.RemoteProvider = (*Provider)(nil)
