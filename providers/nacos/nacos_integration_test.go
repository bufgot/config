package nacos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bufgot/config"
)

func TestNacos_Fetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataId") != "test-app" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":    "nacos-app",
			"version": "2.0",
			"port":    8081,
		})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints: []string{ts.URL},
		AppID:     "test-app",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "nacos" {
		t.Fatalf("Name: expected nacos, got %s", p.Name())
	}

	data, err := p.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if data["name"] != "nacos-app" {
		t.Fatalf("name: expected nacos-app, got %v", data["name"])
	}
	if data["version"] != "2.0" {
		t.Fatalf("version: expected 2.0, got %v", data["version"])
	}
	if data["port"] != float64(8081) {
		t.Fatalf("port: expected 8081, got %v", data["port"])
	}
	t.Logf("nacos fetch OK: %v", data)
}

func TestNacos_Watch(t *testing.T) {
	configVersion := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		configVersion++
		json.NewEncoder(w).Encode(map[string]any{
			"version": configVersion,
			"name":    "nacos-app",
		})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints:   []string{ts.URL},
		AppID:       "test-app",
		SyncInterval: 1,
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-fetch to set baseline hash
	p.Fetch()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	select {
	case data := <-ch:
		t.Logf("watch detected change: %v", data)
		if data["version"] != float64(2) {
			t.Fatalf("version: expected 2, got %v", data["version"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestNacos_WatchCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"key": "val"})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints:   []string{ts.URL},
		AppID:       "test-app",
		SyncInterval: 1,
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	p.Fetch()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cancel()

	timeout := time.After(3 * time.Second)
	closed := false
	for !closed {
		select {
		case _, ok := <-ch:
			if !ok {
				closed = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for channel close after cancel")
		}
	}
	t.Log("nacos watch channel closed after cancel")
}

func TestNacos_Fetch_NotFound(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"http://127.0.0.1:1"},
		AppID:     "test",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = p.Fetch()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	t.Logf("expected error: %v", err)
}

func TestNacos_New_MissingDataId(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"http://127.0.0.1:8848"},
	}

	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for missing dataId")
	}
	t.Logf("expected error: %v", err)
}
