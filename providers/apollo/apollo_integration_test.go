package apollo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bufgot/config"
)

func TestApollo_Fetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"appId":          "test-app",
			"cluster":        "default",
			"namespaceName":  "application",
			"notificationId": 100,
			"configurations": map[string]any{
				"name":    "apollo-app",
				"version": "3.0",
				"port":    8082,
			},
		})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints: []string{ts.URL},
		AppID:     "test-app",
		Cluster:   "default",
		Namespace: "application",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "apollo" {
		t.Fatalf("Name: expected apollo, got %s", p.Name())
	}

	data, err := p.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if data["name"] != "apollo-app" {
		t.Fatalf("name: expected apollo-app, got %v", data["name"])
	}
	if data["version"] != "3.0" {
		t.Fatalf("version: expected 3.0, got %v", data["version"])
	}
	if data["port"] != float64(8082) {
		t.Fatalf("port: expected 8082, got %v", data["port"])
	}
	t.Logf("apollo fetch OK: %v", data)
}

func TestApollo_Watch(t *testing.T) {
	notifID := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		notifID++
		json.NewEncoder(w).Encode(map[string]any{
			"appId":          "test-app",
			"cluster":        "default",
			"namespaceName":  "application",
			"notificationId": notifID,
			"configurations": map[string]any{
				"name":    "apollo-app",
				"version": notifID + 1,
			},
		})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints:    []string{ts.URL},
		AppID:        "test-app",
		Namespace:    "application",
		SyncInterval: 1,
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-fetch to set base notificationID=1
	p.Fetch()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	select {
	case data := <-ch:
		t.Logf("apollo watch detected change: %v", data)
		if data["version"] != float64(3) {
			t.Fatalf("version: expected 3, got %v", data["version"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestApollo_WatchCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"appId":          "test-app",
			"cluster":        "default",
			"namespaceName":  "application",
			"notificationId": 1,
			"configurations": map[string]any{"key": "val"},
		})
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints:    []string{ts.URL},
		AppID:        "test-app",
		Namespace:    "application",
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
	t.Log("apollo watch channel closed after cancel")
}

func TestApollo_New_MissingAppID(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"http://127.0.0.1:8080"},
	}

	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for missing app_id")
	}
	t.Logf("expected error: %v", err)
}

func TestApollo_Fetch_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()

	rc := &config.RemoteConfig{
		Endpoints: []string{ts.URL},
		AppID:     "test-app",
	}

	p, _ := New(rc)
	_, err := p.Fetch()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	t.Logf("expected error: %v", err)
}
