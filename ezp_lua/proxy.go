//go:build lua_bindings
// +build lua_bindings

package ezp_lua

import (
	"ezproxy/handler"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

type luaProxy struct {
	px handler.IProxyContainer
}

func (p *luaProxy) ToTable(l *lua.LState) *lua.LTable {
	tb := l.NewTable()
	addFunction(l, tb, "is_alive", p.bindIsAlive, 0)
	addFunction(l, tb, "send_to_client", p.bindSendToClient, 1)
	addFunction(l, tb, "send_to_server", p.bindSendToServer, 1)
	addFunction(l, tb, "get_id", p.bindGetId, 0)
	addFunction(l, tb, "get_network", p.bindNetwork, 0)
	addFunction(l, tb, "get_server_addr", p.bindGetServerAddr, 0)
	addFunction(l, tb, "get_client_addr", p.bindGetClientAddr, 0)
	addFunction(l, tb, "get_bytes_sent", p.bindGetBytesSent, 0)
	addFunction(l, tb, "get_last_contact", p.bindGetLastContact, 0)
	return tb
}

func (p *luaProxy) bindSendToClient(l *lua.LState) int {
	data := l.CheckString(1)
	err := p.px.SendToClient([]byte(data))
	if err != nil {
		l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
	}
	return 0
}

func (p *luaProxy) bindSendToServer(l *lua.LState) int {
	data := l.CheckString(1)
	err := p.px.SendToServer([]byte(data))
	if err != nil {
		l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
	}
	return 0
}

func (p *luaProxy) bindGetId(l *lua.LState) int {
	n := p.px.GetId()
	l.Push(lua.LNumber(n))
	return 1
}

func (p *luaProxy) bindNetwork(l *lua.LState) int {
	n := p.px.Network()
	l.Push(lua.LString(n))
	return 1
}

func (p *luaProxy) bindIsAlive(l *lua.LState) int {
	n := p.px.IsAlive()
	l.Push(lua.LBool(n))
	return 1
}

func (p *luaProxy) bindGetClientAddr(l *lua.LState) int {
	n := p.px.GetClientAddr().String()
	l.Push(lua.LString(n))
	return 1
}

func (p *luaProxy) bindGetServerAddr(l *lua.LState) int {
	n := p.px.GetServerAddr().String()
	l.Push(lua.LString(n))
	return 1
}

func (p *luaProxy) bindGetBytesSent(l *lua.LState) int {
	n := p.px.GetBytesSent()
	l.Push(lua.LNumber(n))
	return 1
}

func (p *luaProxy) bindGetLastContact(l *lua.LState) int {
	n := p.px.LastContactTimeAgo().Milliseconds()
	l.Push(lua.LNumber(n))
	return 1
}
