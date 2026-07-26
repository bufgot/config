package config

import (
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
