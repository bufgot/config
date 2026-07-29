package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bufgot/config"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Real Nacos 3.x for integration tests.
const (
	testNacosAddr     = "192.168.1.2:2848"
	testNacosGrpcPort = 2849
	testNacosUser     = "nacos"
	testNacosPass     = "nacos"
)

// publishNacosConfig publishes a config to the real Nacos instance.
func publishNacosConfig(t *testing.T, dataID, group, content string) {
	t.Helper()

	cc := constant.ClientConfig{
		NamespaceId:         "",
		TimeoutMs:           10000,
		NotLoadCacheAtStart: true,
		Username:            testNacosUser,
		Password:            testNacosPass,
		LogLevel:            "warn",
	}
	sc := []constant.ServerConfig{{
		IpAddr:   "192.168.1.2",
		Port:     2848,
		GrpcPort: 2849,
	}}

	client, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  &cc,
		ServerConfigs: sc,
	})
	if err != nil {
		t.Fatalf("nacos NewConfigClient: %v", err)
	}

	ok, err := client.PublishConfig(vo.ConfigParam{
		DataId:  dataID,
		Group:   group,
		Content: content,
	})
	if err != nil {
		t.Fatalf("PublishConfig: %v", err)
	}
	if !ok {
		t.Fatal("PublishConfig returned false")
	}
	t.Logf("published to nacos: %s/%s = %s", dataID, group, content)
}

type TestAppConfig struct {
	Name    string `mapstructure:"name" json:"name" yaml:"name"`
	Version string `mapstructure:"version" json:"version" yaml:"version"`
	Port    int    `mapstructure:"port" json:"port" yaml:"port"`
	Etcd    struct {
		Endpoints    []string `mapstructure:"endpoints"`
		Namespace    string   `mapstructure:"namespace"`
		SyncInterval int      `mapstructure:"sync_interval"`
	} `mapstructure:"etcd"`
	Nacos struct {
		Endpoints    []string `mapstructure:"endpoints"`
		AppID        string   `mapstructure:"app_id"`
		SyncInterval int      `mapstructure:"sync_interval"`
	} `mapstructure:"nacos"`
	Apollo struct {
		Endpoints    []string `mapstructure:"endpoints"`
		AppID        string   `mapstructure:"app_id"`
		Namespace    string   `mapstructure:"namespace"`
		SyncInterval int      `mapstructure:"sync_interval"`
	} `mapstructure:"apollo"`
}

// setupMockApollo creates a mock HTTP server for Apollo, returns URL + cleanup.
func setupMockApollo(t *testing.T) (apolloURL string, cleanup func()) {
	t.Helper()

	apolloTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"appId":          "test-app",
			"cluster":        "default",
			"namespaceName":  "application",
			"notificationId": 1,
			"configurations": map[string]any{
				"name":    "apollo-remote",
				"version": "3.0",
				"port":    8082,
			},
		})
	}))

	return apolloTS.URL, func() { apolloTS.Close() }
}

func TestBootstrapRemote_AllThree(t *testing.T) {
	apolloURL, apolloCleanup := setupMockApollo(t)
	defer apolloCleanup()

	// Publish test config to real Nacos
	publishNacosConfig(t, "test-app", "DEFAULT_GROUP",
		`{"name":"nacos-remote","version":"2.0","port":8081}`)
	time.Sleep(500 * time.Millisecond)

	dir := t.TempDir()
	writeFile(t, dir, "app.yml",
		"name: local-app\nversion: 1.0\nport: 3000\n"+
			"etcd:\n  endpoints: [127.0.0.1:2379]\n  namespace: test_app\n"+
			"nacos:\n  endpoints: ["+testNacosAddr+"]\n  app_id: test-app\n  grpc_port: "+strconv.Itoa(testNacosGrpcPort)+"\n  username: "+testNacosUser+"\n  password: "+testNacosPass+"\n"+
			"apollo:\n  endpoints: ["+apolloURL+"]\n  app_id: test-app\n  namespace: application\n")

	app := config.New()
	if err := app.LoadLocal(dir, &TestAppConfig{}); err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}

	BootstrapRemote(app)
	defer app.Close()

	time.Sleep(1 * time.Second)

	if !app.HasProvider("etcd") {
		t.Log("etcd not connected (expected if etcd server not available)")
	}
	if !app.HasProvider("nacos") {
		t.Fatal("nacos provider should have been attached")
	}
	if !app.HasProvider("apollo") {
		t.Fatal("apollo provider should have been attached")
	}

	cfg := app.Get().(*TestAppConfig)
	t.Logf("final config: name=%s version=%s port=%d", cfg.Name, cfg.Version, cfg.Port)
}

func TestBootstrapRemote_None(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: local-only\nversion: 1.0\nport: 4000\n")

	app := config.New()
	if err := app.LoadLocal(dir, &TestAppConfig{}); err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}

	BootstrapRemote(app)
	defer app.Close()

	if app.HasProvider("etcd") || app.HasProvider("nacos") || app.HasProvider("apollo") {
		t.Fatal("no remote providers should be attached when config sections are missing")
	}

	cfg := app.Get().(*TestAppConfig)
	if cfg.Name != "local-only" {
		t.Fatalf("name: expected local-only, got %s", cfg.Name)
	}
	t.Log("no remote providers: OK")
}

func TestBootstrapRemote_Partial(t *testing.T) {
	apolloURL, apolloCleanup := setupMockApollo(t)
	defer apolloCleanup()

	dir := t.TempDir()
	writeFile(t, dir, "app.yml",
		"name: partial\nversion: 1.0\nport: 5000\n"+
			"apollo:\n  endpoints: ["+apolloURL+"]\n  app_id: test-app\n  namespace: application\n")

	app := config.New()
	if err := app.LoadLocal(dir, &TestAppConfig{}); err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}

	BootstrapRemote(app)
	defer app.Close()

	time.Sleep(300 * time.Millisecond)

	if !app.HasProvider("apollo") {
		t.Fatal("apollo provider should be attached")
	}
	if app.HasProvider("nacos") || app.HasProvider("etcd") {
		t.Fatal("only apollo should be attached")
	}
	t.Log("partial bootstrap: only apollo attached")
}

func TestBootstrapRemote_ConfigChangeNotification(t *testing.T) {
	publishNacosConfig(t, "test-app", "DEFAULT_GROUP",
		`{"name":"nacos-remote","version":"1.0","port":8081}`)
	time.Sleep(500 * time.Millisecond)

	dir := t.TempDir()
	writeFile(t, dir, "app.yml",
		"name: local-app\nversion: 1.0\nport: 3000\n"+
			"nacos:\n  endpoints: ["+testNacosAddr+"]\n  app_id: test-app\n  sync_interval: 1\n  grpc_port: "+strconv.Itoa(testNacosGrpcPort)+"\n  username: "+testNacosUser+"\n  password: "+testNacosPass+"\n")

	app := config.New()
	if err := app.LoadLocal(dir, &TestAppConfig{}); err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}

	changed := make(chan struct{}, 1)
	app.OnChange(func(cfg any) {
		t.Logf("OnChange triggered")
		changed <- struct{}{}
	})

	BootstrapRemote(app)
	defer app.Close()

	// Publish updated config to trigger change notification via nacos watch
	time.Sleep(1 * time.Second)
	publishNacosConfig(t, "test-app", "DEFAULT_GROUP",
		`{"name":"nacos-remote","version":"2.0","port":8081}`)

	select {
	case <-changed:
		t.Log("config change notification received")
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for config change notification")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
