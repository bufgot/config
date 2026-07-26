package config

import "context"

// ChangeListener is the type for config change callback functions.
// The cfg parameter is the updated config (same instance as returned by App.Get()).
type ChangeListener func(cfg any)

// RemoteProvider is the interface for remote config providers.
// Implement this interface to integrate any config center (Apollo, Etcd, Consul, Nacos, etc.).
type RemoteProvider interface {
	// Name returns the provider name, used for logging.
	Name() string

	// Fetch pulls full config from remote, returns map[string]any.
	// Config-center-specific settings (URL, AppID, etc.) are held by the implementation at construction time.
	Fetch() (map[string]any, error)

	// Watch starts config change monitoring, sending the latest config to the channel on remote change.
	// The returned channel is consumed by App; stop is signaled via ctx.Done() when App.Close() is called.
	// Implementations should close the channel and exit when ctx is cancelled.
	Watch(ctx context.Context) (<-chan map[string]any, error)
}

// RemoteConfig holds generic remote config center connection info.
// Specific fields are parsed by each provider implementation as needed.
type RemoteConfig struct {
	// Type is the config center type: apollo / etcd / consul / nacos
	Type string `mapstructure:"type" json:"type" yaml:"type"`

	// Endpoints is the list of config center addresses
	Endpoints []string `mapstructure:"endpoints" json:"endpoints" yaml:"endpoints"`

	// AppID is the application identifier (required by Apollo, etc.)
	AppID string `mapstructure:"app_id" json:"app_id" yaml:"app_id"`

	// Cluster is the cluster name (Apollo)
	Cluster string `mapstructure:"cluster" json:"cluster" yaml:"cluster"`

	// Namespace is the namespace
	Namespace string `mapstructure:"namespace" json:"namespace" yaml:"namespace"`

	// SyncInterval is the sync interval in seconds; <=0 uses the provider default
	SyncInterval int `mapstructure:"sync_interval" json:"sync_interval" yaml:"sync_interval"`

	// Extra holds extension fields for provider-specific config
	Extra map[string]any `mapstructure:"extra" json:"extra" yaml:"extra"`
}
