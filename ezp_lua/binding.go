//go:build lua_bindings
// +build lua_bindings

package ezp_lua

import (
	"context"
	"ezproxy/handler"
	"fmt"
	"log/slog"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type LuaRunModes int

func (l LuaRunModes) String() string {
	switch l {
	case LuaRunMain:
		return "LuaRunMain"
	case LuaRunCallback:
		return "LuaRunCallback"
	default:
		return fmt.Sprintf("<Unknown LuaRunMode %d>", int(l))
	}
}

const (
	LuaRunMain LuaRunModes = iota
	LuaRunCallback
)

type luaBindings struct {
	spawner       *luaSpawner
	executionMode LuaRunModes
	logger        *slog.Logger
	path          string
}

func (b *luaBindings) Close() {
	b.spawner.Close()
}

func (b *luaBindings) bindSleep(l *lua.LState) int {
	if l.GetTop() != 1 {
		l.RaiseError(fmt.Sprintf("Expected 1 argument, got %d", l.GetTop()))
		return 0
	}
	ms := l.CheckInt(1)
	if 0 > ms {
		l.ArgError(1, "Must be positive")
		return 0
	}
	time.Sleep(time.Millisecond * time.Duration(ms))
	return 0
}

// TODO: Log the line & file we are logging from
func (b *luaBindings) bindLog(l *lua.LState) int {
	if l.GetTop() != 2 {
		l.RaiseError(fmt.Sprintf("Expected 2 arguments, got %d", l.GetTop()))
		return 0
	}
	/*
		0: DEBUG
		1: INFO
		2: WARN
		3: ERROR
	*/
	lvl := l.CheckInt(1)
	msg := l.CheckString(2)
	switch lvl {
	case 0:
		b.logger.Debug(msg, "Source", "lua", "Path", b.path, "Mode", b.executionMode)
	case 1:
		b.logger.Info(msg, "Source", "lua", "Path", b.path, "Mode", b.executionMode)
	case 2:
		b.logger.Warn(msg, "Source", "lua", "Path", b.path, "Mode", b.executionMode)
	case 3:
		b.logger.Error(msg, "Source", "lua", "Path", b.path, "Mode", b.executionMode)
	default:
		l.ArgError(2, "Argument must be from 0-4")
	}
	return 0
}

func (b *luaBindings) runMain(l *lua.LState, _ context.Context, cancel context.CancelCauseFunc) {
	ezp_main := l.GetGlobal("EzpMain")
	fn, ok := ezp_main.(*lua.LFunction)
	if !ok {
		if ezp_main == nil {
			b.logger.Warn("LUA failed to load, EzpMain is not defined")
			cancel(fmt.Errorf("EzpMain was not found"))
		} else {
			b.logger.Warn("LUA failed to load, EzpMain was not a function", "EzpMain.Type()", ezp_main.Type(), "EzpMain", ezp_main.String())
			cancel(fmt.Errorf("EzpMain was not a function"))
		}
		// We don't restart on this cause nothing will fix it but changing the code, its just burning cycles.
		return
	}
	err := l.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, b.spawner.toTable(l))
	if err != nil {
		b.logger.Warn("Lua failed to execute", "Path", b.path, "Mode", b.executionMode.String(), "Error", err.Error())
		cancel(err)
	}
}

func (b *luaBindings) runCallbacks(l *lua.LState, ctx context.Context, cancel context.CancelCauseFunc) {
	err := runLuaCallback(b, l, b.spawner.spawner, ctx)
	if err != nil {
		b.logger.Warn("Lua failed to execute", "Path", b.path, "Mode", b.executionMode.String(), "Error", err.Error())
		cancel(err)
		return
	}
	cancel(fmt.Errorf("done running"))
	return
}

func (b *luaBindings) run(path string) {
	st := lua.NewState()
	defer st.Close()
	err := st.DoFile(path)
	if err != nil {
		b.logger.Warn("LUA failed to load file", "Error", err, "Path", path)
		// Don't restart - the file doesn't exist.
		return
	}
	st.SetGlobal("sleep", st.NewFunction(b.bindSleep))
	st.SetGlobal("log", st.NewFunction(b.bindLog))
	st.SetGlobal("LEVEL_DEBUG", lua.LNumber(0))
	st.SetGlobal("LEVEL_INFO", lua.LNumber(1))
	st.SetGlobal("LEVEL_WARN", lua.LNumber(2))
	st.SetGlobal("LEVEL_ERROR", lua.LNumber(3))
	ctx, cancel := context.WithCancelCause(b.spawner.spawner.GetContext())
	switch b.executionMode {
	case LuaRunMain:
		b.runMain(st, ctx, cancel)
	case LuaRunCallback:
		b.runCallbacks(st, ctx, cancel)
	default:
		panic("Invalid Lua callback mode")
	}
	return
}

func NewLuaBindingFromFile(spawner handler.IProxySpawner, path string, mode LuaRunModes) error {
	bindings := luaBindings{
		spawner: &luaSpawner{
			spawner: spawner,
			rcvChan: nil,
			ctx:     nil,
			cancel:  nil,
			parent:  nil,
		},
		executionMode: mode,
		logger:        slog.Default(),
		path:          path,
	}
	bindings.spawner.parent = &bindings
	go bindings.run(path)
	return nil
}
