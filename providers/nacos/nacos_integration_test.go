package nacos

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/bufgot/config"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Real Nacos 3.x instance used for integration tests.
const (
	testEndpoint = "192.168.1.2:2848"
	testGrpcPort = 2849
	testUsername = "nacos"
	testPassword = "nacos"
	testDataID   = "bufgot-config-test-local"
	testGroup    = "DEFAULT_GROUP"
)

func testRemoteConfig() *config.RemoteConfig {
	return &config.RemoteConfig{
		Endpoints: []string{testEndpoint},
		AppID:     testDataID,
		Extra: map[string]any{
			"group":    testGroup,
			"grpcPort": testGrpcPort,
			"username": testUsername,
			"password": testPassword,
		},
	}
}

// publishTestConfig publishes a config to Nacos (via gRPC SDK) for test setup.
func publishTestConfig(t *testing.T, content string) {
	t.Helper()

	p, err := New(testRemoteConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ok, err := p.client.PublishConfig(vo.ConfigParam{
		DataId:  testDataID,
		Group:   testGroup,
		Content: content,
	})
	if err != nil {
		t.Fatalf("PublishConfig: %v", err)
	}
	if !ok {
		t.Fatal("PublishConfig returned false")
	}
	t.Logf("published test config: %s", content)
}

func TestNacos_Fetch(t *testing.T) {
	content := `{"name":"nacos-app","version":"2.0","port":8081}`
	publishTestConfig(t, content)

	// Small delay for Nacos to propagate
	time.Sleep(500 * time.Millisecond)

	rc := testRemoteConfig()
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
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
	if p := data["port"]; p != float64(8081) {
		t.Fatalf("port: expected 8081, got %v (%T)", p, p)
	}
	t.Logf("nacos fetch OK: %v", data)
}

func TestNacos_Watch(t *testing.T) {
	// Publish initial config
	publishTestConfig(t, `{"version":1}`)
	time.Sleep(500 * time.Millisecond)

	rc := testRemoteConfig()
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-fetch to establish initial state
	if _, err = p.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Publish updated config after watch starts
	go func() {
		time.Sleep(2 * time.Second)
		publishTestConfig(t, `{"version":2,"updated":true}`)
	}()

	select {
	case data := <-ch:
		t.Logf("watch detected change: %v", data)
		v := data["version"]
		if v == nil {
			t.Fatal("version field missing")
		}
		// version from JSON is float64
		if v != float64(2) {
			t.Fatalf("version: expected 2, got %v (%T)", v, v)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestNacos_WatchCancel(t *testing.T) {
	publishTestConfig(t, `{"key":"val"}`)
	time.Sleep(500 * time.Millisecond)

	rc := testRemoteConfig()
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err = p.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cancel()

	timeout := time.After(5 * time.Second)
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
		Endpoints: []string{"127.0.0.1:1"},
		AppID:     "no-such-app",
		Extra: map[string]any{
			"grpcPort": 1001,
		},
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
		Endpoints: []string{testEndpoint},
	}

	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for missing dataId")
	}
	t.Logf("expected error: %v", err)
}

func TestNacos_Fetch_NonJSON(t *testing.T) {
	content := "hello world, plain text"
	publishTestConfig(t, content)
	time.Sleep(500 * time.Millisecond)

	rc := testRemoteConfig()
	p, err := New(rc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data, err := p.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if data["content"] != content {
		t.Fatalf("content: expected %q, got %v", content, data["content"])
	}
	t.Logf("non-JSON fetch OK: %v", data)
}

func TestNacos_ParseAddr(t *testing.T) {
	tests := []struct {
		addr string
		host string
		port int
	}{
		{"192.168.1.2:8848", "192.168.1.2", 8848},
		{"http://192.168.1.2:8848", "192.168.1.2", 8848},
		{"127.0.0.1:2848", "127.0.0.1", 2848},
	}

	for _, tt := range tests {
		host, port, err := parseAddr(tt.addr)
		if err != nil {
			t.Errorf("parseAddr(%q): %v", tt.addr, err)
			continue
		}
		if host != tt.host || port != tt.port {
			t.Errorf("parseAddr(%q) = (%q,%d), want (%q,%d)",
				tt.addr, host, port, tt.host, tt.port)
		}
	}
}

// unused imports kept for build verification
var _ = json.Marshal
var _ = strconv.Itoa
