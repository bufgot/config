package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/bufgot/config"
)

func TestEtcd_Fetch(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "etcd" {
		t.Fatalf("Name: expected etcd, got %s", p.Name())
	}

	data, err := p.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if data["name"] != "etcd-app" {
		t.Fatalf("name: expected etcd-app, got %v", data["name"])
	}
	// JSON unmarshal of "1.0" → float64(1), "8080" → float64(8080)
	if data["version"] != float64(1) {
		t.Fatalf("version: expected 1.0, got %v (%T)", data["version"], data["version"])
	}
	t.Logf("etcd fetch OK: %v", data)
}

func TestEtcd_Watch(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-fetch to set baseline
	_, err = p.Fetch()
	if err != nil {
		t.Fatalf("pre-fetch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Give watch a moment to establish
	time.Sleep(500 * time.Millisecond)

	// Trigger a change (update port)
	go func() {
		// Use the raw client directly
		p2, _ := New(rc)
		defer p2.client.Close()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel2()
		_, _ = p2.client.Put(ctx2, "/config/test_app/port", "9090")
	}()

	select {
	case data := <-ch:
		t.Logf("watch detected change: %v", data)
		if data["port"] != float64(9090) {
			t.Fatalf("port: expected 9090, got %v (%T)", data["port"], data["port"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}

	// Restore
	p3, _ := New(rc)
	defer p3.client.Close()
	ctx3, cancel3 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel3()
	p3.client.Put(ctx3, "/config/test_app/port", "8080")
}

func TestEtcd_WatchCancel(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}

	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Cancel immediately
	cancel()

	// Channel should close (draining)
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
	t.Log("watch channel closed after cancel")
}
