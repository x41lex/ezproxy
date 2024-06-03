//go:build !lua_bindings && !integration
// +build !lua_bindings,!integration

package main

import "log/slog"

func main() {
	cfg := loadCfg()
	setupLogger(cfg)
	if cfg.Lua.Enable {
		slog.Warn("LUA enabled in config, but this version of EzProxy was built without lua_bindings build tag")
	}
	ps := setupSpawnerAndApi(cfg)
	run(ps)
}
