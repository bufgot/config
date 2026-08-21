package config

import (
	"os"
	"path/filepath"
	"testing"
)

type TestConfig struct {
	Name    string `mapstructure:"name" json:"name" yaml:"name"`
	Version string `mapstructure:"version" json:"version" yaml:"version"`
	Port    int    `mapstructure:"port" json:"port" yaml:"port"`
}

func TestLoadLocal_NoConfigDir(t *testing.T) {
	app := New()
	err := app.LoadLocal("/nonexistent/path", &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal with nonexistent dir should not error: %v", err)
	}
	if !app.IsInitialized() {
		t.Fatal("LoadLocal should set initialized even with no config files")
	}
}

func TestLoadLocal_TwiceIdempotent(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: test\nversion: v1")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("first LoadLocal failed: %v", err)
	}

	// Second call should be no-op
	err = app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("second LoadLocal should be no-op: %v", err)
	}
}

func TestLoadLocal_YAML(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: testapp\nversion: 1.0.0\nport: 8080")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "testapp" {
		t.Fatalf("expected name=testapp, got %s", cfg.Name)
	}
	if cfg.Version != "1.0.0" {
		t.Fatalf("expected version=1.0.0, got %s", cfg.Version)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected port=8080, got %d", cfg.Port)
	}
}

func TestLoadLocal_JSON(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.json", `{"name":"jsonapp","version":"2.0","port":9090}`)

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "jsonapp" {
		t.Fatalf("expected name=jsonapp, got %s", cfg.Name)
	}
}

func TestLoadLocal_MergePriority(t *testing.T) {
	app := New()
	dir := t.TempDir()
	// app.yml has higher priority than app.json
	writeFile(t, dir, "app.json", `{"name":"json-name","version":"json-ver"}`)
	writeFile(t, dir, "app.yml", "name: yml-name")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	// New rule: base files load in extension order yml -> json -> properties,
	// so yml loads first and json loads later, overriding name with json value.
	if cfg.Name != "json-name" {
		t.Fatalf("expected name=json-name (json overrides yml), got %s", cfg.Name)
	}
	if cfg.Version != "json-ver" {
		t.Fatalf("expected version=json-ver, got %s", cfg.Version)
	}
}

func TestLoadLocal_EnvIgnoredByDefault(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: default\nport: 3000")
	writeFile(t, dir, "app-dev.yml", "name: dev-app\nport: 9000")
	writeFile(t, dir, "app-prod.yml", "name: prod-app\nport: 8000")

	// No ENV configured: environment-specific files must NOT be loaded.
	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "default" {
		t.Fatalf("expected name=default (env files ignored), got %s", cfg.Name)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected port=3000 (env files ignored), got %d", cfg.Port)
	}
}

func TestLoadLocal_EnvActivated(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: default\nport: 3000")
	writeFile(t, dir, "app-dev.yml", "name: dev-app\nport: 9000")
	writeFile(t, dir, "app-prod.yml", "name: prod-app\nport: 8000")

	t.Setenv("ENV", "dev")

	// ENV=dev: only app-dev.yml is loaded after base files, overriding values.
	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "dev-app" {
		t.Fatalf("expected name=dev-app (dev env overrides base), got %s", cfg.Name)
	}
	if cfg.Port != 9000 {
		t.Fatalf("expected port=9000 (dev env overrides base), got %d", cfg.Port)
	}
}

func TestLoadLocal_InvalidFile(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "invalid: yaml: :")

	err := app.LoadLocal(dir, &TestConfig{})
	if err == nil {
		t.Fatal("LoadLocal should fail with invalid YAML")
	}
}

func TestScanConfigFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	files, err := scanConfigFiles(dir)
	if err != nil {
		t.Fatalf("scanConfigFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestScanConfigFiles_Nonexistent(t *testing.T) {
	files, err := scanConfigFiles("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("scanConfigFiles should not error for nonexistent dir: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestScanConfigFiles_ParsingAndOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.json", "{}")
	writeFile(t, dir, "app.yml", "")
	writeFile(t, dir, "app-dev.yml", "")
	writeFile(t, dir, "app-prod.yml", "")

	files, err := scanConfigFiles(dir)
	if err != nil {
		t.Fatalf("scanConfigFiles failed: %v", err)
	}

	if len(files) != 4 {
		t.Fatalf("expected 4 candidates, got %d: %v", len(files), files)
	}

	// Candidate metadata: app-dev.yml -> base=app, env=dev; app.yml -> base=app, env=""
	expect := map[string]struct{ base, env string }{
		"app-dev.yml":    {"app", "dev"},
		"app-prod.yml":   {"app", "prod"},
		"app.yml":        {"app", ""},
		"app.json":       {"app", ""},
	}
	for _, f := range files {
		want, ok := expect[f.name]
		if !ok {
			t.Fatalf("unexpected candidate file: %s", f.name)
		}
		if f.base != want.base || f.env != want.env {
			t.Fatalf("file %s: expected base=%q env=%q, got base=%q env=%q",
				f.name, want.base, want.env, f.base, f.env)
		}
	}

	// No active env: only base files load, yml group before json.
	noEnv := buildLoadOrder(files, "")
	expected := []string{"app.yml", "app.json"}
	if len(noEnv) != len(expected) {
		t.Fatalf("expected %d files without env, got %d: %v", len(expected), len(noEnv), noEnv)
	}
	for i, f := range noEnv {
		if f.name != expected[i] {
			t.Fatalf("order[%d]: expected %s, got %s (order: %v)", i, expected[i], f.name, noEnv)
		}
	}

	// Active env=dev: base files first, then matching env file overrides.
	withDev := buildLoadOrder(files, "dev")
	expectedDev := []string{"app.yml", "app.json", "app-dev.yml"}
	if len(withDev) != len(expectedDev) {
		t.Fatalf("expected %d files with env=dev, got %d: %v", len(expectedDev), len(withDev), withDev)
	}
	for i, f := range withDev {
		if f.name != expectedDev[i] {
			t.Fatalf("env order[%d]: expected %s, got %s (order: %v)", i, expectedDev[i], f.name, withDev)
		}
	}
}

func TestScanConfigFiles_IgnoresUnsupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	// All supported extensions are candidates regardless of name prefix.
	writeFile(t, dir, "config.yml", "")
	writeFile(t, dir, "data.json", "")
	writeFile(t, dir, "conf.properties", "")
	// Unsupported extensions are ignored.
	writeFile(t, dir, "readme.md", "")
	writeFile(t, dir, "notes.txt", "")

	files, err := scanConfigFiles(dir)
	if err != nil {
		t.Fatalf("scanConfigFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(files), files)
	}
}

func TestScanConfigFiles_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.Mkdir(subdir, 0755)
	writeFile(t, dir, "app.yml", "")

	files, err := scanConfigFiles(dir)
	if err != nil {
		t.Fatalf("scanConfigFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (directory ignored), got %d", len(files))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// ============================================================
// LoadLocal: already initialized (second call with different files preserved)
// ============================================================

func TestLoadLocal_AlreadyInitialized(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "app.yml", "name: first")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("first LoadLocal failed: %v", err)
	}

	// Wipe dir and write a different file
	os.RemoveAll(dir)
	os.Mkdir(dir, 0755)
	writeFile(t, dir, "app.yml", "name: second")

	// Second call should return immediately (already initialized), not overwrite
	err = app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("second LoadLocal should be no-op: %v", err)
	}
	cfg := app.Get().(*TestConfig)
	if cfg.Name != "first" {
		t.Fatalf("should keep first name, got %s", cfg.Name)
	}
}

// ============================================================
// New rules: no app-* prefix, ext-grouped order, env file filtering
// ============================================================

func TestScanConfigFiles_EnvParsing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "abc-dev.json", "")
	writeFile(t, dir, "abc.yml", "")
	writeFile(t, dir, "abc-dev-prod.properties", "")
	writeFile(t, dir, "abc-.yml", "")

	files, err := scanConfigFiles(dir)
	if err != nil {
		t.Fatalf("scanConfigFiles failed: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 candidates, got %d: %v", len(files), files)
	}

	expect := map[string]struct{ base, env, ext string }{
		"abc-dev.json":          {"abc", "dev", ".json"},
		"abc.yml":               {"abc", "", ".yml"},
		"abc-dev-prod.properties": {"abc-dev", "prod", ".properties"},
		"abc-.yml":              {"abc-", "", ".yml"},
	}
	for _, f := range files {
		want, ok := expect[f.name]
		if !ok {
			t.Fatalf("unexpected candidate file: %s", f.name)
		}
		if f.base != want.base || f.env != want.env || f.ext != want.ext {
			t.Fatalf("file %s: expected base=%q env=%q ext=%q, got base=%q env=%q ext=%q",
				f.name, want.base, want.env, want.ext, f.base, f.env, f.ext)
		}
	}
}

func TestBuildLoadOrder_NoEnv(t *testing.T) {
	cfgs := []configFile{
		{name: "z.yml", base: "z", env: "", ext: ".yml", priority: 1},
		{name: "a.yml", base: "a", env: "", ext: ".yml", priority: 1},
		{name: "m.json", base: "m", env: "", ext: ".json", priority: 2},
		{name: "q.properties", base: "q", env: "", ext: ".properties", priority: 3},
		// environment files must be excluded when no env is active
		{name: "a-dev.yml", base: "a", env: "dev", ext: ".yml", priority: 1},
		{name: "c-prod.yml", base: "c", env: "prod", ext: ".yml", priority: 1},
	}

	order := buildLoadOrder(cfgs, "")
	expected := []string{"a.yml", "z.yml", "m.json", "q.properties"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(order), order)
	}
	for i, f := range order {
		if f.name != expected[i] {
			t.Fatalf("order[%d]: expected %s, got %s (order: %v)", i, expected[i], f.name, order)
		}
	}
}

func TestBuildLoadOrder_WithEnv(t *testing.T) {
	cfgs := []configFile{
		{name: "a.yml", base: "a", env: "", ext: ".yml", priority: 1},
		{name: "m.json", base: "m", env: "", ext: ".json", priority: 2},
		{name: "a-dev.yml", base: "a", env: "dev", ext: ".yml", priority: 1},
		{name: "b-dev.json", base: "b", env: "dev", ext: ".json", priority: 2},
		{name: "c-prod.yml", base: "c", env: "prod", ext: ".yml", priority: 1},
	}

	order := buildLoadOrder(cfgs, "dev")
	expected := []string{"a.yml", "m.json", "a-dev.yml", "b-dev.json"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(order), order)
	}
	for i, f := range order {
		if f.name != expected[i] {
			t.Fatalf("order[%d]: expected %s, got %s (order: %v)", i, expected[i], f.name, order)
		}
	}
}

func TestBuildLoadOrder_EnvCaseInsensitive(t *testing.T) {
	cfgs := []configFile{
		{name: "a.yml", base: "a", env: "", ext: ".yml", priority: 1},
		{name: "a-dev.yml", base: "a", env: "dev", ext: ".yml", priority: 1},
		{name: "b-PROD.json", base: "b", env: "PROD", ext: ".json", priority: 2},
	}

	// activeEnv "DEV" (upper case) must match file env "dev" (lower case).
	order := buildLoadOrder(cfgs, "DEV")
	expected := []string{"a.yml", "a-dev.yml"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(order), order)
	}
	for i, f := range order {
		if f.name != expected[i] {
			t.Fatalf("order[%d]: expected %s, got %s (order: %v)", i, expected[i], f.name, order)
		}
	}

	// activeEnv "prod" (lower case) must match file env "PROD" (upper case).
	order2 := buildLoadOrder(cfgs, "prod")
	if len(order2) != 2 {
		t.Fatalf("expected 2 files for env=prod, got %d: %v", len(order2), order2)
	}
	if order2[1].name != "b-PROD.json" {
		t.Fatalf("expected b-PROD.json appended after base, got %v", order2)
	}
}

func TestResolveActiveEnv(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("ORION_PROFILE", "")
	if v := resolveActiveEnv(); v != "" {
		t.Fatalf("expected empty env, got %q", v)
	}

	// ENV takes priority over ORION_PROFILE
	t.Setenv("ORION_PROFILE", "prod")
	if v := resolveActiveEnv(); v != "prod" {
		t.Fatalf("expected fallback ORION_PROFILE=prod, got %q", v)
	}

	t.Setenv("ENV", "dev")
	if v := resolveActiveEnv(); v != "dev" {
		t.Fatalf("expected ENV=dev to win, got %q", v)
	}
}

func TestLoadLocal_BaseOrderOverride(t *testing.T) {
	app := New()
	dir := t.TempDir()
	// yml group (a then b), then json, then properties last.
	writeFile(t, dir, "a.yml", "name: a-yml\nport: 1")
	writeFile(t, dir, "b.yml", "name: b-yml\nport: 2")
	writeFile(t, dir, "c.json", `{"name":"c-json"}`)
	writeFile(t, dir, "d.properties", "name=d-props")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	// properties group loads last and wins; within yml group b overrides a.
	if cfg.Name != "d-props" {
		t.Fatalf("expected name=d-props (properties overrides all), got %s", cfg.Name)
	}
	if cfg.Port != 2 {
		t.Fatalf("expected port=2 (b.yml overrides a.yml within yml group), got %d", cfg.Port)
	}
}

func TestLoadLocal_EnvCaseInsensitive(t *testing.T) {
	app := New()
	dir := t.TempDir()
	writeFile(t, dir, "abc.yml", "name: base\nport: 3000")
	writeFile(t, dir, "abc-dev.json", `{"name":"dev-json","port":9100}`)

	// ENV=DEV (upper case) must activate the dev environment file (lower case).
	t.Setenv("ENV", "DEV")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "dev-json" {
		t.Fatalf("expected name=dev-json (case-insensitive env match overrides base), got %s", cfg.Name)
	}
	if cfg.Port != 9100 {
		t.Fatalf("expected port=9100 (case-insensitive env match overrides base), got %d", cfg.Port)
	}
}

func TestLoadLocal_BasePlusEnvOverride(t *testing.T) {
	app := New()
	dir := t.TempDir()
	// Base file with a key the env file does NOT define, plus an env file
	// overriding a shared key. Both must be loaded: env overrides shared,
	// base-only key survives.
	writeFile(t, dir, "abc.yml", "name: base\nversion: v-base\nport: 3000")
	writeFile(t, dir, "abc-dev.json", `{"name":"dev-json","port":9100}`)

	t.Setenv("ENV", "dev")

	err := app.LoadLocal(dir, &TestConfig{})
	if err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	cfg := app.Get().(*TestConfig)
	if cfg.Name != "dev-json" {
		t.Fatalf("expected name=dev-json (env file overrides base shared key), got %s", cfg.Name)
	}
	if cfg.Port != 9100 {
		t.Fatalf("expected port=9100 (env file overrides base shared key), got %d", cfg.Port)
	}
	if cfg.Version != "v-base" {
		t.Fatalf("expected version=v-base (base-only key must survive), got %s", cfg.Version)
	}
}
