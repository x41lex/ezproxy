//go:build lua_bindings
// +build lua_bindings

package ezp_lua

import (
	"context"
	"ezproxy/handler"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// Probably log everything here so we can debug the lua code later.

type luaSpawner struct {
	parent  *luaBindings
	spawner handler.IProxySpawner
	rcvChan <-chan handler.PacketChanData
	ctx     context.Context
	cancel  context.CancelFunc
}

func (s *luaSpawner) Close() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.ctx = nil
		s.rcvChan = nil
	}
}

func (s *luaSpawner) toTable(l *lua.LState) *lua.LTable {
	table := l.NewTable()
	addFunction(l, table, "inject_to_server", s.bindInjectToServer, 2)
	addFunction(l, table, "inject_to_client", s.bindInjectToClient, 2)
	addFunction(l, table, "inject_to_both", s.bindInjectToBoth, 2)
	addFunction(l, table, "is_alive", s.bindIsAlive, 0)
	addFunction(l, table, "get_server_address", s.bindGetServerAddress, 0)
	addFunction(l, table, "get_proxy_address", s.bindGetProxyAddress, 0)
	addFunction(l, table, "get_packets", s.bindSetCallback, 1)
	addFunction(l, table, "close", s.bindClose, 1)
	addFunction(l, table, "get_bytes_sent", s.bindGetBytesSent, 0)
	addFunction(l, table, "get_proxy_count", s.bindGetProxyCount, 0)
	addFunction(l, table, "get_proxy", s.bindGetProxy, 1)
	return table
}

func (s *luaSpawner) bindGetProxy(l *lua.LState) int {
	px, err := s.spawner.GetProxy(l.CheckInt(1))
	if err != nil {
		l.RaiseError(err.Error())
		return 0
	}
	lp := luaProxy{px: px}
	l.Push(lp.ToTable(l))
	return 1
}

func (s *luaSpawner) bindInjectToServer(l *lua.LState) int {
	id := l.CheckInt(1)
	data := []byte(l.CheckString(2))
	if id == -1 {
		err := s.spawner.SendToAllServers(data)
		if err != nil {
			l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
		}
	} else {
		px, err := s.spawner.GetProxy(id)
		if err != nil {
			l.RaiseError(err.Error())
			return 0
		}
		err = px.SendToServer(data)
		if err != nil {
			l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
		}
	}
	return 0
}

func (s *luaSpawner) bindInjectToClient(l *lua.LState) int {
	id := l.CheckInt(1)
	data := []byte(l.CheckString(2))
	if id == -1 {
		err := s.spawner.SendToAllClients(data)
		if err != nil {
			l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
		}
	} else {
		px, err := s.spawner.GetProxy(id)
		if err != nil {
			l.RaiseError(err.Error())
			return 0
		}
		err = px.SendToClient(data)
		if err != nil {
			l.RaiseError(fmt.Sprintf("Failed to send: %s", err.Error()))
		}
	}
	return 0
}

func (s *luaSpawner) bindInjectToBoth(l *lua.LState) int {
	id := l.CheckInt(1)
	data := []byte(l.CheckString(2))
	if id == -1 {
		s.spawner.SendToAllClients(data)
	} else {
		px, err := s.spawner.GetProxy(id)
		if err != nil {
			l.RaiseError(err.Error())
			return 0
		}
		px.SendToClient(data)
	}
	return 0
}

func (s *luaSpawner) bindIsAlive(l *lua.LState) int {
	l.Push(lua.LBool(s.spawner.IsAlive()))
	return 1
}

func (s *luaSpawner) bindGetServerAddress(l *lua.LState) int {
	l.Push(lua.LString(s.spawner.GetServerAddr().String()))
	return 1
}

func (s *luaSpawner) bindGetProxyAddress(l *lua.LState) int {
	l.Push(lua.LString(s.spawner.GetProxyAddr().String()))
	return 1
}

// Maybe change to channels?
func (s *luaSpawner) bindSetCallback(l *lua.LState) int {
	f := l.CheckFunction(1)
	if s.rcvChan == nil {
		s.rcvChan, s.ctx, s.cancel = s.spawner.GetRecvChan(s.ctx)
	}
	l.Pop(1)
	s.callbackThread(l, f)
	return 0
}

func (s *luaSpawner) bindClose(l *lua.LState) int {
	v := l.CheckInt(1)
	switch v {
	case -2:
		err := s.spawner.Close()
		s.Close()
		if err != nil {
			l.RaiseError(err.Error())
		}
		return 0
	case -1:
		pxs := s.spawner.GetAllProxies()
		for _, v := range pxs {
			v.Cancel(handler.ErrProxyClosedOk)
		}
		return 0
	default:
		px, err := s.spawner.GetProxy(v)
		if err != nil {
			l.RaiseError(err.Error())
			return 0
		}
		px.Cancel(handler.ErrProxyClosedOk)
		return 0
	}
}

func (s *luaSpawner) bindGetBytesSent(l *lua.LState) int {
	l.Push(lua.LNumber(s.spawner.GetBytesSent()))
	return 1
}

func (s *luaSpawner) bindGetProxyCount(l *lua.LState) int {
	l.Push(lua.LNumber(len(s.spawner.GetAllProxies())))
	return 1
}

func (s *luaSpawner) callbackThread(l *lua.LState, f *lua.LFunction) {
	for {
		select {
		case data := <-s.rcvChan:
			tb := packetDataToTable(l, &data)
			err := l.CallByParam(lua.P{
				Fn:      f,
				NRet:    1,
				Protect: true,
			}, s.toTable(l), tb)
			if err != nil {
				s.parent.logger.Warn("LUA callback failed", "Error", err.Error())
				s.cancel()
				continue
			}
			v := l.CheckBool(1)
			if v {
				s.parent.logger.Info("LUA uninstalling callback")
				s.cancel()
			}
		case <-s.ctx.Done():
			s.cancel()
			s.cancel = nil
			s.ctx = nil
			s.rcvChan = nil
			return
		}
	}
}
