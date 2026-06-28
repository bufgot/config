package config

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// UseRemote connects to a remote config center.
//
// provider is a RemoteProvider constructed from remote config connection info parsed from local config.
// After calling this method, App will:
//  1. Asynchronously fetch remote config once and merge into existing config
//  2. Start a watcher goroutine to continuously monitor remote changes
//
// Requires LoadLocal to have been called first.
func (a *App) UseRemote(provider RemoteProvider) error {
	a.mu.Lock()
	if !a.initialized {
		a.mu.Unlock()
		return fmt.Errorf("config: LoadLocal must be called before UseRemote")
	}
	a.remoteProvider = provider
	a.mu.Unlock()

	log.Printf("[config] remote provider [%s] attached", provider.Name())

	// Async fetch remote config
	go a.fetchRemoteConfig(provider)

	// Start watcher
	ch, err := provider.Watch()
	if err != nil {
		return fmt.Errorf("config: start watch on %s: %w", provider.Name(), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.watcherCancel = cancel
	a.mu.Unlock()

	go a.watchRemoteLoop(ctx, ch, provider.Name())

	return nil
}

// fetchRemoteConfig fetches remote config and merges it.
func (a *App) fetchRemoteConfig(provider RemoteProvider) {
	log.Printf("[config] fetching remote config from %s...", provider.Name())

	data, err := provider.Fetch()
	if err != nil {
		log.Printf("[config] fetch remote config from %s failed: %v", provider.Name(), err)
		return
	}

	a.mu.Lock()
	flattened := flattenMap(data)
	for k, v := range flattened {
		a.viper.Set(k, v)
	}
	if err := a.viper.Unmarshal(a.raw); err != nil {
		log.Printf("[config] unmarshal after remote fetch: %v", err)
		a.mu.Unlock()
		return
	}
	log.Printf("[config] remote config from %s merged (%d keys)", provider.Name(), len(flattened))
	a.mu.Unlock()

	a.notifyChange()
}

// watchRemoteLoop continuously monitors remote config changes.
func (a *App) watchRemoteLoop(ctx context.Context, ch <-chan map[string]any, providerName string) {
	log.Printf("[config] watcher started for %s", providerName)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[config] watcher for %s stopped", providerName)
			return

		case data, ok := <-ch:
			if !ok {
				log.Printf("[config] watcher channel for %s closed", providerName)
				return
			}

			log.Printf("[config] remote config changed from %s, applying...", providerName)

			a.mu.Lock()
			flattened := flattenMap(data)
			for k, v := range flattened {
				a.viper.Set(k, v)
			}
			if err := a.viper.Unmarshal(a.raw); err != nil {
				log.Printf("[config] unmarshal after remote change: %v", err)
				a.mu.Unlock()
				continue
			}
			a.mu.Unlock()

			a.notifyChange()
		}
	}
}

// flattenMap flattens a nested map into "."-separated keys (viper style).
func flattenMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	flattenMapRecurse(m, "", result)
	return result
}

func flattenMapRecurse(m map[string]any, prefix string, result map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenMapRecurse(val, key, result)
		default:
			result[key] = v
		}
	}
}

// ParseRemoteConfig parses RemoteConfig from the loaded viper instance.
// keyPrefix is the top-level key for remote config in the config file, e.g. "apollo" or "etcd".
// Returns nil if the keyPrefix config does not exist or lacks endpoints.
func ParseRemoteConfig(v *viper.Viper, keyPrefix string) *RemoteConfig {
	if !v.IsSet(keyPrefix) {
		return nil
	}

	rc := &RemoteConfig{}
	sub := v.Sub(keyPrefix)
	if sub == nil {
		return nil
	}
	if err := sub.Unmarshal(rc); err != nil {
		log.Printf("[config] parse remote config [%s]: %v", keyPrefix, err)
		return nil
	}

	if rc.Type == "" {
		rc.Type = keyPrefix
	}

	// Compatibility for single ip field (apollo config often uses ip field instead of endpoints array)
	if len(rc.Endpoints) == 0 && v.IsSet(keyPrefix+".ip") {
		ip := v.GetString(keyPrefix + ".ip")
		if ip != "" {
			rc.Endpoints = []string{ip}
		}
	}

	if len(rc.Endpoints) == 0 {
		return nil
	}

	return rc
}
