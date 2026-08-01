package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/bufgot/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ============================================================
// Unit tests (no external server needed)
// ============================================================

func TestNew_NilConfig(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("New should fail with nil config")
	}
}

func TestNew_EmptyEndpoints(t *testing.T) {
	_, err := New(&config.RemoteConfig{})
	if err == nil {
		t.Fatal("New should fail with empty endpoints")
	}
}

func TestKeyPrefix_Namespace(t *testing.T) {
	p := &Provider{cfg: &config.RemoteConfig{Namespace: "myapp"}}
	if got := p.keyPrefix(); got != "/config/myapp/" {
		t.Fatalf("expected /config/myapp/, got %s", got)
	}
}

func TestKeyPrefix_AppID(t *testing.T) {
	p := &Provider{cfg: &config.RemoteConfig{AppID: "myapp"}}
	if got := p.keyPrefix(); got != "/config/myapp/" {
		t.Fatalf("expected /config/myapp/, got %s", got)
	}
}

func TestKeyPrefix_Default(t *testing.T) {
	p := &Provider{cfg: &config.RemoteConfig{}}
	if got := p.keyPrefix(); got != "/config/" {
		t.Fatalf("expected /config/, got %s", got)
	}
}

func TestName(t *testing.T) {
	p := &Provider{}
	if p.Name() != "etcd" {
		t.Fatalf("expected etcd, got %s", p.Name())
	}
}

func TestNew_InvalidEndpoint(t *testing.T) {
	// endpoints with invalid format → clientv3.New should fail
	_, err := New(&config.RemoteConfig{
		Endpoints: []string{"http://127.0.0.1:1"},
	})
	if err != nil {
		t.Logf("expected error: %v", err)
	} else {
		t.Log("etcd.New succeeded despite invalid endpoint (may be valid for client creation)")
	}
}

// ============================================================
// Integration tests (require etcd at 192.168.1.2:2479)
// ============================================================

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

// ============================================================
// Additional coverage tests
// ============================================================

func TestEtcd_Fetch_PlainString(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}

	// Put a plain string value (not JSON)
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p.client.Put(ctx, "/config/test_app/plain_key", "this-is-plain-text")

	// Fetch and verify the plain string is returned as-is
	data, err := p.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if data["plain_key"] != "this-is-plain-text" {
		t.Fatalf("expected 'this-is-plain-text', got %v", data["plain_key"])
	}
	t.Logf("plain string fetch OK: %v", data)
}

func TestEtcd_Fetch_ConnectError(t *testing.T) {
	// Use an etcd client with a closed connection to trigger Get error
	p := &Provider{
		cfg: &config.RemoteConfig{Namespace: "test"},
	}
	// Create client, then close it immediately
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"192.168.1.2:2479"},
		DialTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Skipf("cannot create client: %v", err)
	}
	cli.Close()
	p.client = cli

	_, err = p.Fetch()
	if err == nil {
		t.Fatal("expected error from closed client")
	}
	t.Logf("expected fetch error: %v", err)
}

func TestEtcd_Watch_ChannelFull(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Trigger multiple changes quickly to potentially fill the channel
	go func() {
		p2, _ := New(rc)
		defer p2.client.Close()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel2()
		p2.client.Put(ctx2, "/config/test_app/key1", "v1")
		time.Sleep(100 * time.Millisecond)
		p2.client.Put(ctx2, "/config/test_app/key2", "v2")
		time.Sleep(100 * time.Millisecond)
		p2.client.Put(ctx2, "/config/test_app/key3", "v3")
	}()

	// Just consume one event to verify it works
	select {
	case data := <-ch:
		t.Logf("watch event: %v", data)
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}

	// Restore
	p3, _ := New(rc)
	defer p3.client.Close()
	ctx3, cancel3 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel3()
	p3.client.Put(ctx3, "/config/test_app/key1", "8080")
}

// TestEtcd_Watch_ChannelDrop triggers rapid changes to hit the "channel full" default branch.
func TestEtcd_Watch_ChannelDrop(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"192.168.1.2:2479"},
		Namespace: "test_app",
	}
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Trigger two rapid changes - first fills channel, second should hit "channel full"
	go func() {
		p2, _ := New(rc)
		defer p2.client.Close()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel2()
		p2.client.Put(ctx2, "/config/test_app/ch1", "a")
		time.Sleep(50 * time.Millisecond)
		p2.client.Put(ctx2, "/config/test_app/ch1", "b")
	}()

	// Consume one to allow test to complete
	select {
	case data := <-ch:
		t.Logf("watch event: %v", data)
	case <-time.After(8 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
}
