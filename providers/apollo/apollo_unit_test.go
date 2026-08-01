package apollo

import (
	"testing"

	"github.com/bufgot/config"
)

func TestGetIntFromMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want int
		wantOk bool
	}{
		{"not found", map[string]any{}, "x", 0, false},
		{"nil map", nil, "x", 0, false},
		{"float64", map[string]any{"n": float64(42)}, "n", 42, true},
		{"int", map[string]any{"n": 100}, "n", 100, true},
		{"int64", map[string]any{"n": int64(200)}, "n", 200, true},
		{"string (not int)", map[string]any{"n": "abc"}, "n", 0, false},
		{"bool (not int)", map[string]any{"n": true}, "n", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := getIntFromMap(tt.m, tt.key)
			if ok != tt.wantOk || got != tt.want {
				t.Fatalf("getIntFromMap() = (%d, %v), want (%d, %v)", got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

func TestName(t *testing.T) {
	p := &Provider{}
	if p.Name() != "apollo" {
		t.Fatalf("expected 'apollo', got %q", p.Name())
	}
}

func TestNamespace(t *testing.T) {
	p := &Provider{cfg: &config.RemoteConfig{}}
	got := p.namespace()
	if got != "application" {
		t.Fatalf("namespace() = %q, want 'application'", got)
	}

	p2 := &Provider{cfg: &config.RemoteConfig{Namespace: "my-ns"}}
	got2 := p2.namespace()
	if got2 != "my-ns" {
		t.Fatalf("namespace() = %q, want 'my-ns'", got2)
	}
}

func TestNew_NilRC(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil RemoteConfig")
	}
	t.Logf("expected error: %v", err)
}

func TestNew_EmptyEndpoints(t *testing.T) {
	rc := &config.RemoteConfig{AppID: "test"}
	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for empty endpoints")
	}
	t.Logf("expected error: %v", err)
}

func TestNew_DefaultInterval(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"http://127.0.0.1:8080"},
		AppID:     "test",
	}
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.cfg.SyncInterval != 0 {
		t.Fatalf("expected SyncInterval 0 (unchanged), got %d", p.cfg.SyncInterval)
	}
}
