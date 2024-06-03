//go:build lua_bindings
// +build lua_bindings

package ezp_lua

import (
	"ezproxy/handler"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func packetDataToTable(l *lua.LState, data *handler.PacketChanData) *lua.LTable {
	tb := l.NewTable()
	tb.RawSetString("serverbound", lua.LBool(data.Flags.IsServerbound()))
	tb.RawSetString("injected", lua.LBool(data.Flags.IsInjected()))
	tb.RawSetString("flags", lua.LNumber(int(data.Flags)))
	tb.RawSetString("source", lua.LString(data.Source.String()))
	tb.RawSetString("dest", lua.LString(data.Dest.String()))
	tb.RawSetString("proxy_id", lua.LNumber(data.ProxyId))
	tb.RawSetString("data", lua.LString(string(data.Data)))
	return tb
}

func addFunction(l *lua.LState, tb *lua.LTable, name string, fn lua.LGFunction, args int) {
	tb.RawSetString(name, l.NewFunction(func(l *lua.LState) int {
		if l.GetTop() != args {
			l.RaiseError(fmt.Sprintf("Expected %d arguments, got %d", args, l.GetTop()))
			return 0
		}
		return fn(l)
	}))
}
