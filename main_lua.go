//go:build lua_bindings && !integration
// +build lua_bindings,!integration

package main

import (
	"ezproxy/ezp_lua"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, "Compiled with LUA support, this is a experimental feature and will likely change in the future, it is also very likely to cause crashes with lua\n")
	cfg := loadCfg()
	setupLogger(cfg)
	slog.Default().Warn("Compiled with LUA support, this is a experimental feature and will likely change in the future, it is also very likely to cause crashes if your lua code is bad")
	ps := setupSpawnerAndApi(cfg)
	if cfg.Lua.Enable {
		mode := ezp_lua.LuaRunMain
		switch cfg.Lua.Mode {
		case "main":
			mode = ezp_lua.LuaRunMain
		case "callback":
			mode = ezp_lua.LuaRunCallback
		default:
			fmt.Fprintf(os.Stderr, "Invalid Lua.Mode, must be 'main' or 'callback', was %s\n", cfg.Lua.Mode)
			return
		}
		err := ezp_lua.NewLuaBindingFromFile(ps, cfg.Lua.Path, mode)
		if err != nil {
			slog.Default().Warn("LUA bindings failed to execute", "Error", err, "Path", "test.lua")
			return
		}
	}
	run(ps)
}
