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

// filePriority defines the loading order of config file extensions
// (smaller values load first): yml -> json -> properties.
var filePriority = map[string]int{
	".yml":        1,
	".yaml":       1, // .yaml is treated as the yml group
	".json":       2,
	".properties": 3,
}

// configFile describes a candidate config file found under a config dir.
type configFile struct {
	path     string // full path
	name     string // file name
	base     string // base name without extension (env suffix removed)
	env      string // environment name; empty means a base file
	ext      string // lower-cased extension, e.g. ".yml"
	priority int    // extension group priority for ordering
}

// resolveActiveEnv returns the active environment name. It reads the ENV
// environment variable first, falling back to ORION_PROFILE. An empty result
// means no environment is activated and environment-specific files are ignored.
func resolveActiveEnv() string {
	if v := os.Getenv("ENV"); v != "" {
		return v
	}
	return os.Getenv("ORION_PROFILE")
}

// LoadLocal loads local config files from localDir and deserializes into cfg.
//
// Loading rules:
//   - Any file whose extension is .yml / .yaml / .json / .properties is treated
//     as a config file, regardless of its name prefix.
//   - Base files (no -{env} suffix, e.g. abc.yml) are loaded first: grouped by
//     extension in order yml -> json -> properties, and alphabetically by file
//     name within each group. Later files override earlier ones on merge.
//   - Environment-specific files named {base}-{env}.{ext} (e.g. abc-dev.json)
//     are ignored by default. When an active environment (ENV, fallback
//     ORION_PROFILE) is set, only environment files matching that env name
//     (case-insensitively) are loaded after the base files, so they override
//     base configuration.
//
// cfg must be a pointer, used for viper.Unmarshal deserialization.
func (a *App) LoadLocal(localDir string, cfg any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.initialized {
		return nil
	}

	a.raw = cfg

	// Collect all candidate config files and resolve final load order.
	cfgs, err := scanConfigFiles(localDir)
	if err != nil {
		return fmt.Errorf("config: scan config files in %s: %w", localDir, err)
	}

	order := buildLoadOrder(cfgs, resolveActiveEnv())

	if len(order) == 0 {
		log.Printf("[config] no config files found in %s, using defaults", localDir)
	} else {
		for _, f := range order {
			log.Printf("[config] loading: %s", f.path)
			v := viper.New()
			v.SetConfigFile(f.path)
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("config: read %s: %w", f.path, err)
			}
			if err := a.viper.MergeConfigMap(v.AllSettings()); err != nil {
				return fmt.Errorf("config: merge %s: %w", f.path, err)
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

// scanConfigFiles scans dir and returns all candidate config files, unsorted.
// A file is a candidate iff its extension is .yml / .yaml / .json / .properties,
// regardless of name prefix. Files named {base}-{env}.{ext} are tagged with
// their environment name; the part before the last "-" becomes the file's base.
func scanConfigFiles(dir string) ([]configFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfgs []configFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		pri, ok := filePriority[ext]
		if !ok {
			continue
		}

		base := name[:len(name)-len(ext)]
		env := ""
		// {base}-{env}.{ext}: env is the part after the last "-" (must be non-empty).
		if i := strings.LastIndex(base, "-"); i >= 0 && i < len(base)-1 {
			env = base[i+1:]
			base = base[:i]
		}

		cfgs = append(cfgs, configFile{
			path:     filepath.Join(dir, name),
			name:     name,
			base:     base,
			env:      env,
			ext:      ext,
			priority: pri,
		})
	}
	return cfgs, nil
}

// buildLoadOrder resolves the final loading order of candidate files:
//  1. ALL base files (no env suffix) first, grouped by extension
//     (yml -> json -> properties) and alphabetically by file name within each group;
//  2. If activeEnv is non-empty, environment files whose env matches activeEnv
//     (case-insensitive) are appended after all base files (same ordering),
//     so they override the base configuration. Environment files for other
//     environments are never loaded.
func buildLoadOrder(cfgs []configFile, activeEnv string) []configFile {
	var bases, envs []configFile
	for _, c := range cfgs {
		switch {
		case c.env == "":
			bases = append(bases, c)
		case activeEnv != "" && strings.EqualFold(c.env, activeEnv):
			envs = append(envs, c)
		}
	}
	sortConfigFiles(bases)
	sortConfigFiles(envs)
	return append(bases, envs...)
}

// sortConfigFiles orders files by extension priority (yml -> json -> properties),
// then alphabetically by file name.
func sortConfigFiles(fs []configFile) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].priority != fs[j].priority {
			return fs[i].priority < fs[j].priority
		}
		return fs[i].name < fs[j].name
	})
}
