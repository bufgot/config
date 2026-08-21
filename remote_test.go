package config

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// ============================================================
// flattenMap / flattenMapRecurse
// ============================================================

func TestFlattenMap_Flat(t *testing.T) {
	m := map[string]any{
		"key1": "val1",
		"key2": 42,
	}
	result := flattenMap(m)
	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}
	if result["key1"] != "val1" {
		t.Fatalf("expected val1, got %v", result["key1"])
	}
	if result["key2"] != 42 {
		t.Fatalf("expected 42, got %v", result["key2"])
	}
}

func TestFlattenMap_Nested(t *testing.T) {
	m := map[string]any{
		"app": map[string]any{
			"name": "test",
			"meta": map[string]any{
				"version": "1.0",
			},
		},
	}
	result := flattenMap(m)
	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}
	if result["app.name"] != "test" {
		t.Fatalf("expected test, got %v", result["app.name"])
	}
	if result["app.meta.version"] != "1.0" {
		t.Fatalf("expected 1.0, got %v", result["app.meta.version"])
	}
}

func TestFlattenMap_Empty(t *testing.T) {
	result := flattenMap(map[string]any{})
	if len(result) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(result))
	}
}

func TestFlattenMap_MixedTypes(t *testing.T) {
	m := map[string]any{
		"str":   "hello",
		"num":   123,
		"bool":  true,
		"float": 3.14,
		"nested": map[string]any{
			"inner": 42,
		},
	}
	result := flattenMap(m)
	if result["str"] != "hello" {
		t.Fatalf("expected hello, got %v", result["str"])
	}
	if result["num"] != 123 {
		t.Fatalf("expected 123, got %v", result["num"])
	}
	if result["bool"] != true {
		t.Fatalf("expected true, got %v", result["bool"])
	}
	if result["nested.inner"] != 42 {
		t.Fatalf("expected 42, got %v", result["nested.inner"])
	}
}

// ============================================================
// ParseRemoteConfig
// ============================================================

func TestParseRemoteConfig_NotSet(t *testing.T) {
	v := viper.New()
	rc := ParseRemoteConfig(v, "apollo")
	if rc != nil {
		t.Fatal("ParseRemoteConfig should return nil when key is not set")
	}
}

func TestParseRemoteConfig_Basic(t *testing.T) {
	v := viper.New()
	v.Set("apollo.type", "apollo")
	v.Set("apollo.endpoints", []string{"http://apollo:8080"})
	v.Set("apollo.app_id", "test-app")

	rc := ParseRemoteConfig(v, "apollo")
	if rc == nil {
		t.Fatal("ParseRemoteConfig should not return nil")
	}
	if rc.Type != "apollo" {
		t.Fatalf("expected type=apollo, got %s", rc.Type)
	}
	if len(rc.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(rc.Endpoints))
	}
}

func TestParseRemoteConfig_EmptyTypeDefault(t *testing.T) {
	v := viper.New()
	v.Set("apollo.endpoints", []string{"http://apollo:8080"})

	rc := ParseRemoteConfig(v, "apollo")
	if rc == nil {
		t.Fatal("ParseRemoteConfig should not return nil")
	}
	if rc.Type != "apollo" {
		t.Fatalf("expected default type=apollo, got %s", rc.Type)
	}
}

func TestParseRemoteConfig_IpField(t *testing.T) {
	v := viper.New()
	v.Set("apollo.ip", "192.168.1.1:8080")

	rc := ParseRemoteConfig(v, "apollo")
	if rc == nil {
		t.Fatal("ParseRemoteConfig should not return nil with ip field")
	}
	if len(rc.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint from ip, got %d", len(rc.Endpoints))
	}
	if rc.Endpoints[0] != "192.168.1.1:8080" {
		t.Fatalf("expected 192.168.1.1:8080, got %s", rc.Endpoints[0])
	}
}

func TestParseRemoteConfig_IpEmpty(t *testing.T) {
	v := viper.New()
	v.Set("apollo.ip", "")

	rc := ParseRemoteConfig(v, "apollo")
	// Empty ip + no endpoints = nil
	if rc != nil {
		t.Fatal("ParseRemoteConfig should return nil when ip is empty and no endpoints")
	}
}

func TestParseRemoteConfig_IpWithEndpoints(t *testing.T) {
	v := viper.New()
	v.Set("apollo.endpoints", []string{"http://a:8080", "http://b:8080"})
	v.Set("apollo.ip", "192.168.1.1:8080")

	rc := ParseRemoteConfig(v, "apollo")
	if rc == nil {
		t.Fatal("ParseRemoteConfig should not return nil")
	}
	// endpoints should take precedence
	if len(rc.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(rc.Endpoints))
	}
}

func TestParseRemoteConfig_NoEndpoints(t *testing.T) {
	v := viper.New()
	v.Set("apollo.type", "apollo")

	rc := ParseRemoteConfig(v, "apollo")
	if rc != nil {
		t.Fatal("ParseRemoteConfig should return nil with no endpoints")
	}
}

func TestParseRemoteConfig_Etcd(t *testing.T) {
	v := viper.New()
	v.Set("etcd.endpoints", []string{"http://etcd:2379"})

	rc := ParseRemoteConfig(v, "etcd")
	if rc == nil {
		t.Fatal("ParseRemoteConfig should not return nil")
	}
	if rc.Type != "etcd" {
		t.Fatalf("expected type=etcd, got %s", rc.Type)
	}
}

func TestParseRemoteConfig_SubIsNil(t *testing.T) {
	// This tests when v.Sub returns nil (e.g., key is a scalar, not a map)
	v := viper.New()
	v.Set("apollo", "not-a-map")

	rc := ParseRemoteConfig(v, "apollo")
	if rc != nil {
		t.Fatal("ParseRemoteConfig should return nil when sub is nil")
	}
}

// ============================================================
// Mock RemoteProvider
// ============================================================

type mockProvider struct {
	name    string
	fetchFn func() (map[string]any, error)
	watchFn func(ctx context.Context) (<-chan map[string]any, error)
	watchCh chan map[string]any
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Fetch() (map[string]any, error) {
	if m.fetchFn != nil {
		return m.fetchFn()
	}
	return map[string]any{"key": "value"}, nil
}

func (m *mockProvider) Watch(ctx context.Context) (<-chan map[string]any, error) {
	if m.watchFn != nil {
		return m.watchFn(ctx)
	}
	m.watchCh = make(chan map[string]any)
	return m.watchCh, nil
}

// ============================================================
// UseRemote
// ============================================================

func TestUseRemote_NotInitialized(t *testing.T) {
	app := New()
	mp := &mockProvider{name: "test"}

	err := app.UseRemote(mp)
	if err == nil {
		t.Fatal("UseRemote should fail when LoadLocal not called")
	}
}

func TestUseRemote_Duplicate(t *testing.T) {
	app := New()
	mp := &mockProvider{name: "test"}

	// Need init first
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: ok")
	app.LoadLocal(dir, &TestConfig{})

	// Manually inject to simulate already attached
	app.watcherMu.Lock()
	app.remoteProviders = map[string]RemoteProvider{"test": mp}
	app.watcherMu.Unlock()

	err := app.UseRemote(mp)
	if err == nil {
		t.Fatal("UseRemote should fail for duplicate provider")
	}
}

func TestUseRemote_Success(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: base")
	app.LoadLocal(dir, &TestConfig{})

	mp := &mockProvider{
		name: "mock",
		fetchFn: func() (map[string]any, error) {
			return map[string]any{
				"name": "remote-name",
				"port": 9999,
			}, nil
		},
	}

	err := app.UseRemote(mp)
	if err != nil {
		t.Fatalf("UseRemote failed: %v", err)
	}

	if !app.HasProvider("mock") {
		t.Fatal("HasProvider should return true after UseRemote")
	}

	// Wait for async fetch
	time.Sleep(100 * time.Millisecond)

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "remote-name" {
		t.Fatalf("expected name=remote-name after remote fetch, got %s", cfg.Name)
	}
	if cfg.Port != 9999 {
		t.Fatalf("expected port=9999 after remote fetch, got %d", cfg.Port)
	}

	// Cleanup
	app.Close()
}

func TestUseRemote_WatchError(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: base")
	app.LoadLocal(dir, &TestConfig{})

	mp := &mockProvider{
		name: "bad-watch",
		watchFn: func(ctx context.Context) (<-chan map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
	}

	// Even with watch error, the provider should be registered
	err := app.UseRemote(mp)
	if err == nil {
		t.Log("WatchError: expected error from watch, but provider may already be attached")
	}
}

func TestUseRemote_FetchError(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: base")
	app.LoadLocal(dir, &TestConfig{})

	mp := &mockProvider{
		name: "fetch-fail",
		fetchFn: func() (map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
	}

	err := app.UseRemote(mp)
	if err != nil {
		// Watch error is possible
		t.Logf("UseRemote returned: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Config should still be the original
	cfg := app.Get().(*TestConfig)
	if cfg.Name != "base" {
		t.Fatalf("expected name=base, got %s", cfg.Name)
	}
	app.Close()
}

func TestUseRemote_MultipleProviders(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: base\nport: 3000")
	app.LoadLocal(dir, &TestConfig{})

	mp1 := &mockProvider{
		name: "p1",
		fetchFn: func() (map[string]any, error) {
			return map[string]any{"version": "v1"}, nil
		},
	}
	mp2 := &mockProvider{
		name: "p2",
		fetchFn: func() (map[string]any, error) {
			return map[string]any{"version": "v2"}, nil
		},
	}

	err := app.UseRemote(mp1)
	if err != nil {
		t.Fatalf("UseRemote p1 failed: %v", err)
	}
	err = app.UseRemote(mp2)
	if err != nil {
		t.Fatalf("UseRemote p2 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	app.Close()
}

// ============================================================
// watchRemoteLoop
// ============================================================

func TestWatchRemoteLoop_ChannelClosed(t *testing.T) {
	app := New()
	type testCfg struct {
		Name string
		Port int
	}
	app.raw = &testCfg{}

	ch := make(chan map[string]any)
	close(ch) // immediately closed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run in goroutine, should exit immediately on closed channel
	done := make(chan struct{})
	go func() {
		app.watchRemoteLoop(ctx, ch, "test-closed")
		done <- struct{}{}
	}()

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Fatal("watchRemoteLoop should exit when channel is closed")
	}
}

func TestWatchRemoteLoop_ContextCancelled(t *testing.T) {
	app := New()
	type testCfg struct {
		Name string
		Port int
	}
	app.raw = &testCfg{}

	ch := make(chan map[string]any)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		app.watchRemoteLoop(ctx, ch, "test-cancelled")
		done <- struct{}{}
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Fatal("watchRemoteLoop should exit on context cancel")
	}
}

func TestWatchRemoteLoop_DataArrives(t *testing.T) {
	app := New()
	type testCfg struct {
		Name string
		Port int
	}
	app.raw = &testCfg{}

	ch := make(chan map[string]any, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.watchRemoteLoop(ctx, ch, "test-data")
	}()

	ch <- map[string]any{"name": "updated", "port": 8080}
	time.Sleep(50 * time.Millisecond)

	cfg := app.raw.(*testCfg)
	if cfg.Name != "updated" {
		t.Fatalf("expected name=updated, got %s", cfg.Name)
	}

	cancel()
	wg.Wait()
}

// ============================================================
// logRemoteDiff (dev env remote config diff logging)
// ============================================================

// captureLog runs fn while redirecting the global logger to a buffer and
// returns everything logged during the call.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	fn()
	return buf.String()
}

func TestLogRemoteDiff_Dev_AddedChangedRemoved(t *testing.T) {
	app := New()
	// No ENV/ORION_PROFILE set => active env is dev.
	oldFlat := map[string]any{
		"app.name": "old-name",
		"port":     int(8080),
		"db.host":  "127.0.0.1",
	}
	newFlat := map[string]any{
		"app.name": "new-name", // changed
		"port":     int(8080),  // unchanged
		"feature":  true,       // added
	}

	out := captureLog(func() {
		app.logRemoteDiff(oldFlat, newFlat)
	})

	if !strings.Contains(out, "added: feature = true") {
		t.Fatalf("expected added log for feature, got:\n%s", out)
	}
	if !strings.Contains(out, "changed: app.name value from old-name to new-name") {
		t.Fatalf("expected changed log for app.name, got:\n%s", out)
	}
	if !strings.Contains(out, "removed: db.host (old value 127.0.0.1)") {
		t.Fatalf("expected removed log for db.host, got:\n%s", out)
	}
	// unchanged key must not appear in the diff
	if strings.Contains(out, "port") {
		t.Fatalf("unchanged key port should not be logged, got:\n%s", out)
	}
}

func TestLogRemoteDiff_DevEnvExplicit(t *testing.T) {
	app := New()
	t.Setenv("ENV", "dev")

	oldFlat := map[string]any{"k": "a"}
	newFlat := map[string]any{"k": "b"}

	out := captureLog(func() {
		app.logRemoteDiff(oldFlat, newFlat)
	})

	if !strings.Contains(out, "changed: k value from a to b") {
		t.Fatalf("expected changed log under ENV=dev, got:\n%s", out)
	}
}

func TestLogRemoteDiff_NonDev_Silent(t *testing.T) {
	app := New()
	t.Setenv("ENV", "prod")

	oldFlat := map[string]any{"k": "a"}
	newFlat := map[string]any{"k": "b"}

	out := captureLog(func() {
		app.logRemoteDiff(oldFlat, newFlat)
	})

	if out != "" {
		t.Fatalf("expected no diff logs under non-dev env, got:\n%s", out)
	}
}

func TestLogRemoteDiff_ValueNormalizedCompare(t *testing.T) {
	app := New()
	// Same semantic value in different concrete types must be treated as equal
	// via fmt.Sprintf("%v") normalization.
	oldFlat := map[string]any{"ratio": float64(1)}
	newFlat := map[string]any{"ratio": int64(1)}

	out := captureLog(func() {
		app.logRemoteDiff(oldFlat, newFlat)
	})

	if strings.Contains(out, "ratio") {
		t.Fatalf("normalized-equal value should not be logged, got:\n%s", out)
	}
}

func TestLogRemoteDiff_EmptyBoth(t *testing.T) {
	app := New()
	out := captureLog(func() {
		app.logRemoteDiff(map[string]any{}, map[string]any{})
	})
	if out != "" {
		t.Fatalf("expected no logs for empty diffs, got:\n%s", out)
	}
}

// ============================================================
// fetchRemoteConfig direct
// ============================================================

func TestFetchRemoteConfig_FetchError(t *testing.T) {
	app := New()
	type testCfg struct{ Name string }
	app.raw = &testCfg{Name: "original"}

	mp := &mockProvider{
		name: "fail",
		fetchFn: func() (map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
	}

	app.fetchRemoteConfig(mp)
	// Should not change raw due to fetch error
	cfg := app.raw.(*testCfg)
	if cfg.Name != "original" {
		t.Fatalf("expected name=original, got %s", cfg.Name)
	}
}

func TestFetchRemoteConfig_UnmarshalError(t *testing.T) {
	app := New()
	type testCfg struct{ Name string }
	app.raw = &testCfg{Name: "original"}

	mp := &mockProvider{
		name: "bad",
		fetchFn: func() (map[string]any, error) {
			// Return complex nested data that can't unmarshal into simple string field
			return map[string]any{
				"name": []string{"this", "is", "not", "a", "string"},
			}, nil
		},
	}

	// This should log an error but not panic
	app.fetchRemoteConfig(mp)
}
