package nacos

import (
	"context"
	"testing"

	"github.com/bufgot/config"
)

func TestProvider_Name(t *testing.T) {
	p := &Provider{}
	if p.Name() != "nacos" {
		t.Fatalf("expected 'nacos', got %q", p.Name())
	}
}

func TestDataID(t *testing.T) {
	tests := []struct {
		name string
		rc   *config.RemoteConfig
		want string
	}{
		{
			name: "nil Extra, use AppID",
			rc:   &config.RemoteConfig{AppID: "myapp"},
			want: "myapp",
		},
		{
			name: "empty Extra, use AppID",
			rc:   &config.RemoteConfig{AppID: "myapp", Extra: map[string]any{}},
			want: "myapp",
		},
		{
			name: "Extra with dataId",
			rc:   &config.RemoteConfig{AppID: "myapp", Extra: map[string]any{"dataId": "custom-data-id"}},
			want: "custom-data-id",
		},
		{
			name: "Extra with empty dataId, fallback to AppID",
			rc:   &config.RemoteConfig{AppID: "myapp", Extra: map[string]any{"dataId": ""}},
			want: "myapp",
		},
		{
			name: "Extra with non-string dataId, fallback to AppID",
			rc:   &config.RemoteConfig{AppID: "myapp", Extra: map[string]any{"dataId": 123}},
			want: "myapp",
		},
		{
			name: "Extra with snake_case data_id",
			rc:   &config.RemoteConfig{AppID: "myapp", Extra: map[string]any{"data_id": "snake-id"}},
			want: "myapp", // dataID only checks camelCase key, not snake_case
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataID(tt.rc)
			if got != tt.want {
				t.Fatalf("dataID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroup(t *testing.T) {
	tests := []struct {
		name string
		rc   *config.RemoteConfig
		want string
	}{
		{
			name: "nil Extra",
			rc:   &config.RemoteConfig{},
			want: "DEFAULT_GROUP",
		},
		{
			name: "Extra with group",
			rc:   &config.RemoteConfig{Extra: map[string]any{"group": "custom-group"}},
			want: "custom-group",
		},
		{
			name: "Extra with empty group",
			rc:   &config.RemoteConfig{Extra: map[string]any{"group": ""}},
			want: "DEFAULT_GROUP",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := group(tt.rc)
			if got != tt.want {
				t.Fatalf("group() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtraStr(t *testing.T) {
	tests := []struct {
		name string
		rc   *config.RemoteConfig
		key  string
		want string
	}{
		{
			name: "nil Extra",
			rc:   &config.RemoteConfig{},
			key:  "username",
			want: "",
		},
		{
			name: "camelCase key",
			rc:   &config.RemoteConfig{Extra: map[string]any{"username": "admin"}},
			key:  "username",
			want: "admin",
		},
		{
			name: "snake_case key fallback",
			rc:   &config.RemoteConfig{Extra: map[string]any{"user_name": "snake-admin"}},
			key:  "userName",
			want: "snake-admin",
		},
		{
			name: "camelCase preferred over snake_case",
			rc:   &config.RemoteConfig{Extra: map[string]any{"userName": "camel", "user_name": "snake"}},
			key:  "userName",
			want: "camel",
		},
		{
			name: "empty string value",
			rc:   &config.RemoteConfig{Extra: map[string]any{"username": ""}},
			key:  "username",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extraStr(tt.rc, tt.key)
			if got != tt.want {
				t.Fatalf("extraStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtraInt(t *testing.T) {
	tests := []struct {
		name string
		rc   *config.RemoteConfig
		key  string
		want int
	}{
		{
			name: "nil Extra",
			rc:   &config.RemoteConfig{},
			key:  "grpcPort",
			want: 0,
		},
		{
			name: "int value",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpcPort": 8848}},
			key:  "grpcPort",
			want: 8848,
		},
		{
			name: "float64 value",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpcPort": float64(9848)}},
			key:  "grpcPort",
			want: 9848,
		},
		{
			name: "zero int",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpcPort": 0}},
			key:  "grpcPort",
			want: 0,
		},
		{
			name: "zero float64",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpcPort": float64(0)}},
			key:  "grpcPort",
			want: 0,
		},
		{
			name: "string value (ignored)",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpcPort": "8848"}},
			key:  "grpcPort",
			want: 0,
		},
		{
			name: "snake_case key",
			rc:   &config.RemoteConfig{Extra: map[string]any{"grpc_port": 8848}},
			key:  "grpcPort",
			want: 8848,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extraInt(tt.rc, tt.key)
			if got != tt.want {
				t.Fatalf("extraInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:    "host:port",
			addr:    "192.168.1.2:8848",
			wantHost: "192.168.1.2",
			wantPort: 8848,
		},
		{
			name:    "http:// prefix",
			addr:    "http://192.168.1.2:8848",
			wantHost: "192.168.1.2",
			wantPort: 8848,
		},
		{
			name:    "https:// prefix",
			addr:    "https://192.168.1.2:8848",
			wantHost: "192.168.1.2",
			wantPort: 8848,
		},
		{
			name:    "localhost",
			addr:    "localhost:8848",
			wantHost: "localhost",
			wantPort: 8848,
		},
		{
			name:   "invalid port",
			addr:   "192.168.1.2:abc",
			wantErr: true,
		},
		{
			name:   "missing port",
			addr:   "192.168.1.2",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseAddr(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got host=%q port=%d", host, port)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAddr(%q): %v", tt.addr, err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Fatalf("parseAddr(%q) = (%q,%d), want (%q,%d)",
					tt.addr, host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestKeyVariants(t *testing.T) {
	tests := []struct {
		camel string
		want  []string
	}{
		{"", []string{"", ""}},
		{"grpcPort", []string{"grpcPort", "grpc_port"}},
		{"dataId", []string{"dataId", "data_id"}},
		{"userName", []string{"userName", "user_name"}},
	}
	for _, tt := range tests {
		got := keyVariants(tt.camel)
		if len(got) != len(tt.want) {
			t.Fatalf("keyVariants(%q) len=%d, want len=%d", tt.camel, len(got), len(tt.want))
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("keyVariants(%q)[%d] = %q, want %q", tt.camel, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseContent_JSON(t *testing.T) {
	result := parseContent(`{"key":"val","num":42}`)
	if result["key"] != "val" {
		t.Fatalf("expected val, got %v", result["key"])
	}
	if result["num"] != float64(42) {
		t.Fatalf("expected 42, got %v (%T)", result["num"], result["num"])
	}
}

func TestParseContent_NonJSON(t *testing.T) {
	result := parseContent("plain text")
	if result["content"] != "plain text" {
		t.Fatalf("expected 'plain text', got %v", result["content"])
	}
}

// Ensure interface compliance
func TestProvider_ImplementsRemoteProvider(t *testing.T) {
	var _ config.RemoteProvider = (*Provider)(nil)
}

// ============================================================
// New error paths
// ============================================================

func TestNew_MissingEndpoints(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil RemoteConfig")
	}
	t.Logf("nil RemoteConfig: %v", err)

	_, err = New(&config.RemoteConfig{Endpoints: []string{}})
	if err == nil {
		t.Fatal("expected error for empty endpoints")
	}
	t.Logf("empty endpoints: %v", err)
}

func TestNew_MissingDataID(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"127.0.0.1:8848"},
		AppID:     "",
	}
	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for missing dataId")
	}
	t.Logf("missing dataId: %v", err)
}

func TestNew_InvalidEndpoint(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"bad:address:here"},
		AppID:     "myapp",
	}
	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
	t.Logf("invalid endpoint: %v", err)
}

func TestNew_InvalidPort(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"127.0.0.1:abc"},
		AppID:     "myapp",
	}
	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
	t.Logf("invalid port: %v", err)
}

func TestNew_MissingPort(t *testing.T) {
	rc := &config.RemoteConfig{
		Endpoints: []string{"127.0.0.1"},
		AppID:     "myapp",
	}
	_, err := New(rc)
	if err == nil {
		t.Fatal("expected error for missing port")
	}
	t.Logf("missing port: %v", err)
}

// ============================================================
// Watch double-call error path
// ============================================================

func TestWatch_DoubleCall(t *testing.T) {
	// Use a real Nacos connection (integration test has one available)
	rc := testRemoteConfig()
	p, err := New(rc)
	if err != nil {
		t.Skipf("Nacos not available, skipping Watch test: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = p.Watch(ctx)
	if err != nil {
		t.Fatalf("first Watch: %v", err)
	}

	_, err = p.Watch(ctx)
	if err == nil {
		t.Fatal("expected error on second Watch")
	}
	t.Logf("double Watch: %v", err)
}
