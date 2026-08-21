package config

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// UseRemote connects to a remote config center.
//
// Multiple providers may be attached (e.g. etcd + nacos + apollo).
// After calling this method, App will:
//  1. Asynchronously fetch remote config once and merge into existing config
//  2. Start a watcher goroutine to continuously monitor remote changes
//
// Requires LoadLocal to have been called first.
// Returns error if a provider with the same name is already attached.
func (a *App) UseRemote(provider RemoteProvider) error {
	name := provider.Name()

	a.watcherMu.Lock()
	if a.remoteProviders == nil {
		a.remoteProviders = make(map[string]RemoteProvider)
		a.watcherCancels = make(map[string]context.CancelFunc)
	}
	if _, exists := a.remoteProviders[name]; exists {
		a.watcherMu.Unlock()
		return fmt.Errorf("config: remote provider [%s] already attached", name)
	}

	a.mu.RLock()
	initialized := a.initialized
	a.mu.RUnlock()
	if !initialized {
		a.watcherMu.Unlock()
		return fmt.Errorf("config: LoadLocal must be called before UseRemote")
	}

	a.remoteProviders[name] = provider
	a.watcherMu.Unlock()

	log.Printf("[config] remote provider [%s] attached", name)

	// Async fetch remote config
	go a.fetchRemoteConfig(provider)

	// Start watcher
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := provider.Watch(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("config: start watch on %s: %w", name, err)
	}

	a.watcherMu.Lock()
	a.watcherCancels[name] = cancel
	a.watcherMu.Unlock()

	go a.watchRemoteLoop(ctx, ch, name)

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
			oldFlat := flattenMap(a.viper.AllSettings())
			for k, v := range flattened {
				a.viper.Set(k, v)
			}
			a.logRemoteDiff(oldFlat, flattened)
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

// isDevEnv reports whether the active environment is dev. resolveActiveEnv()
// returns the ENV variable, falling back to ORION_PROFILE; an empty result
// means no environment is activated, which is treated as dev.
func isDevEnv() bool {
	env := resolveActiveEnv()
	return env == "" || strings.EqualFold(env, "dev")
}

// logRemoteDiff logs the per-key difference between the previous flattened
// config snapshot (oldFlat) and the newly received remote config (newFlat).
//
//   - a key present in newFlat but missing in oldFlat is logged as added
//   - a key present in both with a different value is logged as changed
//     ("value from X to Y"), values compared via fmt.Sprintf("%v")
//   - a key present in oldFlat but missing in newFlat is logged as removed
//
// Because viper.Set never deletes keys that disappear from the remote config,
// removed keys stay in viper but are still reported as removed by the remote.
// Logging only happens when the active environment is dev.
func (a *App) logRemoteDiff(oldFlat, newFlat map[string]any) {
	if !isDevEnv() {
		return
	}

	for _, k := range sortedFlatKeys(newFlat) {
		oldV, existed := oldFlat[k]
		switch {
		case !existed:
			log.Printf("[config] remote config key added: %s = %v", k, fmt.Sprintf("%v", newFlat[k]))
		case fmt.Sprintf("%v", oldV) != fmt.Sprintf("%v", newFlat[k]):
			log.Printf("[config] remote config key changed: %s value from %v to %v",
				k, fmt.Sprintf("%v", oldV), fmt.Sprintf("%v", newFlat[k]))
		}
	}

	for _, k := range sortedFlatKeys(oldFlat) {
		if _, ok := newFlat[k]; !ok {
			log.Printf("[config] remote config key removed: %s (old value %v)", k, fmt.Sprintf("%v", oldFlat[k]))
		}
	}
}

// sortedFlatKeys returns the keys of m in deterministic (lexicographic) order.
func sortedFlatKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
