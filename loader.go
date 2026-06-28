package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// filePriority defines the loading priority of config files (smaller values = higher priority).
var filePriority = map[string]int{
	".yml":        1,
	".yaml":       1,
	".json":       2,
	".properties": 3,
}

// LoadLocal loads local config files from localDir and deserializes into cfg.
//
// Loading rules:
//   - Scans all app-* prefixed .yml / .yaml / .json / .properties files under localDir
//   - For the same environment, yml takes priority over json, json over properties
//   - Default files without env suffix (e.g. app.yml) are loaded last as fallback (append mode)
//   - Multiple files are merged into viper in priority order
//
// cfg must be a pointer, used for viper.Unmarshal deserialization.
func (a *App) LoadLocal(localDir string, cfg any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.initialized {
		return nil
	}

	a.raw = cfg

	// Collect all config files, sort by priority
	files, err := scanConfigFiles(localDir)
	if err != nil {
		return fmt.Errorf("config: scan config files in %s: %w", localDir, err)
	}

	if len(files) == 0 {
		log.Printf("[config] no config files found in %s, using defaults", localDir)
	} else {
		for _, f := range files {
			log.Printf("[config] loading: %s", f)
			v := viper.New()
			v.SetConfigFile(f)
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("config: read %s: %w", f, err)
			}
			if err := a.viper.MergeConfigMap(v.AllSettings()); err != nil {
				return fmt.Errorf("config: merge %s: %w", f, err)
			}
		}
	}

	if err := a.viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("config: unmarshal: %w", err)
	}

	a.initialized = true
	log.Println("[config] local configuration loaded successfully")
	return nil
}

// scanConfigFiles scans the directory and returns config file paths sorted by priority.
//
// Sorting rules:
//  1. Group by environment name (no env name = "default")
//  2. Within each group, sort by extension priority (yml > json > properties)
//  3. Default group (no env name) is placed last
func scanConfigFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type configFile struct {
		path     string
		env      string // env name, empty string means default
		ext      string
		priority int
	}

	var cfgs []configFile

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		base := name[:len(name)-len(ext)]

		// Only process files with app-* prefix
		if !strings.HasPrefix(base, "app") {
			continue
		}

		// Only process supported extensions
		pri, ok := filePriority[ext]
		if !ok {
			continue
		}

		// Parse env name: app-dev.yml → env=dev; app.yml → env=""
		env := ""
		rest := strings.TrimPrefix(base, "app")
		if strings.HasPrefix(rest, "-") {
			env = rest[1:] // remove leading "-"
		}

		cfgs = append(cfgs, configFile{
			path:     filepath.Join(dir, name),
			env:      env,
			ext:      ext,
			priority: pri,
		})
	}

	// Sort: default group (env="") last, others by env + priority
	sort.Slice(cfgs, func(i, j int) bool {
		a, b := cfgs[i], cfgs[j]
		// Default group goes last
		if a.env == "" && b.env != "" {
			return false
		}
		if a.env != "" && b.env == "" {
			return true
		}
		// Within same group, sort by extension priority
		if a.env == b.env {
			return a.priority < b.priority
		}
		// Different env names sorted alphabetically
		return a.env < b.env
	})

	paths := make([]string, len(cfgs))
	for i, c := range cfgs {
		paths[i] = c.path
	}
	return paths, nil
}
