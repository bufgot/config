// Package bootstrap provides automatic remote config center connection.
//
// BootstrapRemote scans the loaded local config for etcd / nacos / apollo sections,
// connects to each configured center, and starts change monitoring.
//
// Usage:
//
//	app := config.New()
//	app.LoadLocal("local", &MyConfig{})
//	bootstrap.BootstrapRemote(app)
//	defer app.Close()
package bootstrap

import (
	"log"

	"github.com/bufgot/config"
	"github.com/bufgot/config/providers/apollo"
	"github.com/bufgot/config/providers/etcd"
	"github.com/bufgot/config/providers/nacos"
)

// BootstrapRemote scans the App's loaded local config for etcd / nacos / apollo sections,
// connects to each configured center, and starts change monitoring.
//
// Each provider is optional: if no config section is found, it is silently skipped.
// Connection errors are logged but non-fatal; subsequent providers still attempt to connect.
func BootstrapRemote(app *config.App) {
	// --- etcd ---
	if rc := config.ParseRemoteConfig(app.Viper(), "etcd"); rc != nil {
		p, err := etcd.New(rc)
		if err != nil {
			log.Printf("[config] etcd init failed: %v", err)
		} else if err := app.UseRemote(p); err != nil {
			log.Printf("[config] etcd attach failed: %v", err)
		}
	}

	// --- nacos ---
	if rc := config.ParseRemoteConfig(app.Viper(), "nacos"); rc != nil {
		p, err := nacos.New(rc)
		if err != nil {
			log.Printf("[config] nacos init failed: %v", err)
		} else if err := app.UseRemote(p); err != nil {
			log.Printf("[config] nacos attach failed: %v", err)
		}
	}

	// --- apollo ---
	if rc := config.ParseRemoteConfig(app.Viper(), "apollo"); rc != nil {
		p, err := apollo.New(rc)
		if err != nil {
			log.Printf("[config] apollo init failed: %v", err)
		} else if err := app.UseRemote(p); err != nil {
			log.Printf("[config] apollo attach failed: %v", err)
		}
	}
}
