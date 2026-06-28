// Package config provides general-purpose application configuration management.
//
// Core features:
//   - Local config loading: load app-*.{yml,json,properties} from local/ directory by priority (yml > json > properties)
//   - Remote config support: integrate Apollo, Etcd, and other config centers via the RemoteProvider interface
//   - Config change listening: Observer pattern, notifies all registered listeners on config change
//   - Concurrency safety: global config reads/writes protected by sync.RWMutex
//
// Usage example:
//
//	// 1. Load local config
//	app := config.New()
//	if err := app.LoadLocal(&MyConfig{}); err != nil { ... }
//
//	// 2. Connect to remote config center (optional)
//	provider := config.NewApolloProvider(config.ApolloConfig{...})
//	app.UseRemote(provider)
//
//	// 3. Register config change listener
//	app.OnChange(func(newCfg config.AppConfig) {
//	    log.Println("config changed, re-applying...")
//	})
//
//	// 4. Get config
//	cfg := app.Get().(*MyConfig)
package config

import (
	"fmt"
	"log"
	"sync"

	"github.com/spf13/viper"
)

// App is the top-level configuration management instance, encapsulating viper, remote provider, and change listener mechanism.
// Typically, only one App instance is created per application.
type App struct {
	mu       sync.RWMutex
	viper    *viper.Viper
	raw      any // User-provided struct pointer, target for viper.Unmarshal

	remoteProvider RemoteProvider
	watcherCancel  func() // context cancel function to stop the watcher

	listeners   []ChangeListener
	listenerMu  sync.RWMutex

	initialized bool
}

// New creates a new App instance.
func New() *App {
	return &App{
		viper: viper.New(),
	}
}

// Get returns the deserialized config struct pointer (thread-safe).
// Callers need to type-assert, e.g. cfg := app.Get().(*MyConfig)
func (a *App) Get() any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.raw
}

// MustGet is the same as Get, but panics if config has not been initialized.
func (a *App) MustGet() any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.raw == nil {
		panic("config: not initialized, call LoadLocal or LoadLocalWithViper first")
	}
	return a.raw
}

// Viper returns the underlying viper instance for advanced users.
func (a *App) Viper() *viper.Viper {
	return a.viper
}

// IsInitialized returns whether the config has been initialized.
func (a *App) IsInitialized() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.initialized
}

// OnChange registers a config change listener. Whenever config changes (local hot-reload or remote push),
// all listeners are invoked asynchronously.
func (a *App) OnChange(listener ChangeListener) {
	a.listenerMu.Lock()
	defer a.listenerMu.Unlock()
	a.listeners = append(a.listeners, listener)
}

// notifyChange notifies all listeners that config has changed. Each listener is called in a separate goroutine.
func (a *App) notifyChange() {
	a.listenerMu.RLock()
	listeners := make([]ChangeListener, len(a.listeners))
	copy(listeners, a.listeners)
	a.listenerMu.RUnlock()

	cfg := a.Get()
	for _, l := range listeners {
		go func(listener ChangeListener) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[config] panic in change listener: %v", r)
				}
			}()
			listener(cfg)
		}(l)
	}
}

// Close shuts down config management, stops the remote watcher, and clears listeners.
func (a *App) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.watcherCancel != nil {
		a.watcherCancel()
		a.watcherCancel = nil
	}

	a.listenerMu.Lock()
	a.listeners = nil
	a.listenerMu.Unlock()

	a.initialized = false
	a.raw = nil
	log.Println("[config] resources closed")
}

// reloadFromViper re-deserializes from viper into user struct and notifies listeners.
func (a *App) reloadFromViper() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.raw == nil {
		return fmt.Errorf("config: raw target not set, call LoadLocal first")
	}

	if err := a.viper.Unmarshal(a.raw); err != nil {
		return fmt.Errorf("config: unmarshal on reload: %w", err)
	}

	return nil
}
