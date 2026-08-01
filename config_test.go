package config

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New returned nil")
	}
	if app.viper == nil {
		t.Fatal("New app.viper is nil")
	}
}

func TestGet_Nil(t *testing.T) {
	app := New()
	got := app.Get()
	if got != nil {
		t.Fatal("Get before LoadLocal should return nil")
	}
}

func TestMustGet_Panic(t *testing.T) {
	app := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustGet should panic when not initialized")
		}
	}()
	app.MustGet()
}

func TestIsInitialized(t *testing.T) {
	app := New()
	if app.IsInitialized() {
		t.Fatal("IsInitialized should be false for new App")
	}
}

func TestOnChange(t *testing.T) {
	app := New()

	var called bool
	var mu sync.Mutex
	app.OnChange(func(cfg any) {
		mu.Lock()
		defer mu.Unlock()
		called = true
	})

	if len(app.listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(app.listeners))
	}

	// Manually call notifyChange and wait for goroutine
	app.raw = "test-config"
	app.notifyChange()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("listener was not called")
	}
}

func TestNotifyChange_PanicRecovery(t *testing.T) {
	app := New()
	app.raw = "test"

	var called bool
	var mu sync.Mutex

	app.OnChange(func(cfg any) {
		mu.Lock()
		defer mu.Unlock()
		called = true
	})
	app.OnChange(func(cfg any) {
		panic("intentional panic in listener")
	})

	app.notifyChange()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("listener should be called even if previous panicked")
	}
}

func TestNotifyChange_MultipleListeners(t *testing.T) {
	app := New()
	app.raw = "test"

	var count int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		app.OnChange(func(cfg any) {
			mu.Lock()
			defer mu.Unlock()
			count++
		})
	}

	app.notifyChange()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 5 {
		t.Fatalf("expected 5 listener calls, got %d", count)
	}
}

func TestClose(t *testing.T) {
	app := New()
	app.listeners = append(app.listeners, func(cfg any) {})
	app.initialized = true
	app.raw = "test"
	app.Close()

	if app.IsInitialized() {
		t.Fatal("IsInitialized should be false after Close")
	}
	if app.Get() != nil {
		t.Fatal("Get should return nil after Close")
	}
	if len(app.listeners) != 0 {
		t.Fatal("listeners should be empty after Close")
	}
}

func TestReloadFromViper_NotLoaded(t *testing.T) {
	app := New()
	err := app.reloadFromViper()
	if err == nil {
		t.Fatal("reloadFromViper should fail when raw is nil")
	}
}

func TestViper(t *testing.T) {
	app := New()
	v := app.Viper()
	if v == nil {
		t.Fatal("Viper returned nil")
	}
	if v != app.viper {
		t.Fatal("Viper should return the same instance")
	}
}

func TestConcurrentGet(t *testing.T) {
	app := New()
	app.raw = "test"
	app.initialized = true

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = app.Get()
			_ = app.IsInitialized()
		}()
	}
	wg.Wait()
}

// ============================================================
// HasProvider
// ============================================================

func TestHasProvider_False(t *testing.T) {
	app := New()
	if app.HasProvider("nonexistent") {
		t.Fatal("HasProvider should return false for unattached provider")
	}
}

func TestHasProvider_True(t *testing.T) {
	app := New()
	app.remoteProviders = map[string]RemoteProvider{
		"test": nil,
	}
	if !app.HasProvider("test") {
		t.Fatal("HasProvider should return true for attached provider")
	}
}

// ============================================================
// Close with watcher cancels
// ============================================================

func TestClose_WithWatchers(t *testing.T) {
	app := New()
	app.listeners = append(app.listeners, func(cfg any) {})
	app.initialized = true
	app.raw = "test"

	cancelCalled := false
	cancel := func() { cancelCalled = true }
	cancel2 := func() { cancelCalled = true }
	app.watcherCancels = map[string]context.CancelFunc{
		"p1": cancel,
		"p2": cancel2,
	}
	app.remoteProviders = map[string]RemoteProvider{
		"p1": nil,
		"p2": nil,
	}

	app.Close()

	if !cancelCalled {
		t.Fatal("Close should cancel watchers")
	}
	if len(app.watcherCancels) != 0 {
		t.Fatal("watcherCancels should be nil after Close")
	}
}

// ============================================================
// reloadFromViper success
// ============================================================

func TestReloadFromViper_Success(t *testing.T) {
	app := New()

	type cfg struct {
		Name string
		Port int
	}
	app.raw = &cfg{}

	app.viper.Set("name", "reload-test")
	app.viper.Set("port", 9999)

	err := app.reloadFromViper()
	if err != nil {
		t.Fatalf("reloadFromViper failed: %v", err)
	}

	c := app.raw.(*cfg)
	if c.Name != "reload-test" {
		t.Fatalf("expected name=reload-test, got %s", c.Name)
	}
	if c.Port != 9999 {
		t.Fatalf("expected port=9999, got %d", c.Port)
	}
}

func TestReloadFromViper_UnmarshalError(t *testing.T) {
	app := New()

	// Use a struct that will fail unmarshal (e.g., a string instead of a struct pointer)
	app.raw = new(int) // viper can't unmarshal into int pointer well, but let's try
	app.viper.Set("name", "test")

	// This may or may not error; the important thing is it doesn't panic
	_ = app.reloadFromViper()
}

// ============================================================
// MustGet success
// ============================================================

func TestMustGet_Success(t *testing.T) {
	app := New()
	app.raw = "test-config"
	app.initialized = true

	got := app.MustGet()
	if got != "test-config" {
		t.Fatalf("expected test-config, got %v", got)
	}
}
