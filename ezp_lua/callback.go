//go:build lua_bindings
// +build lua_bindings

package ezp_lua

import (
	"context"
	"errors"
	"ezproxy/handler"

	lua "github.com/yuin/gopher-lua"
)

type luaCallback struct {
	state        *lua.LState
	parent       *luaBindings
	onPacket     *lua.LFunction
	filterPacket *lua.LFunction
	rcv          <-chan handler.PacketChanData
	rcvCtx       context.Context
	luaCancel    context.CancelFunc
	luaCtx       context.Context
}

func (c *luaCallback) handleFilters(data []byte, flags handler.CapFlags, proxy handler.IProxyContainer) bool {
	source := proxy.GetServerAddr()
	dest := proxy.GetClientAddr()
	if flags.IsServerbound() {
		source = proxy.GetClientAddr()
		dest = proxy.GetServerAddr()
	}
	tb := packetDataToTable(c.state, &handler.PacketChanData{
		Flags:   flags,
		Source:  source,
		Dest:    dest,
		Data:    data,
		ProxyId: proxy.GetId(),
	})
	err := c.state.CallByParam(lua.P{
		Fn:      c.filterPacket,
		NRet:    1,
		Protect: true,
	}, c.parent.spawner.toTable(c.state), tb)
	if err != nil {
		c.parent.logger.Warn("LUA callback failed", "Error", err.Error(), "Func", "EzpFilter")
		c.luaCancel()
		return true
	}
	rawRet := c.state.Get(-1)
	c.state.Pop(1)
	v, ok := rawRet.(lua.LBool)
	if !ok {
		c.parent.logger.Warn("LUA return was not a bool", "Func", "EzpFilter", "ReturnType", rawRet)
		c.luaCancel()
		return true
	}
	return bool(v)
}

func (c *luaCallback) sendOnPacket(pkt *handler.PacketChanData) bool {
	tb := packetDataToTable(c.state, pkt)
	err := c.state.CallByParam(lua.P{
		Fn:      c.onPacket,
		NRet:    1,
		Protect: true,
	}, c.parent.spawner.toTable(c.state), tb)
	if err != nil {
		c.parent.logger.Warn("LUA callback failed", "Error", err.Error(), "Func", "EzpOnPacket")
		c.luaCancel()
		return true
	}
	rawRet := c.state.Get(-1)
	c.state.Pop(1)
	v, ok := rawRet.(lua.LBool)
	if !ok {
		c.parent.logger.Warn("LUA return was not a bool", "Func", "EzpOnPacket", "ReturnType", rawRet)
		c.luaCancel()
		return true
	}
	return bool(v)
}

func (c *luaCallback) run() {
	for {
		select {
		case pkt := <-c.rcv:
			if c.onPacket != nil {
				if c.sendOnPacket(&pkt) {
					c.parent.logger.Info("LUA callback uninstalled safely", "Func", "EzpOnPacket")
					// Cancel the rcvChan
					c.luaCancel()
					return
				}
			}
		case <-c.rcvCtx.Done():
			c.parent.logger.Debug("Cancelling Lua context, rcvCtx died", "Error", c.rcvCtx.Err().Error())
			c.luaCancel()
			return
		case <-c.luaCtx.Done():
			c.parent.logger.Debug("Lua context died", "Error", c.luaCtx.Err().Error())
			return
		}
	}
}

func runLuaCallback(parent *luaBindings, l *lua.LState, sp handler.IProxySpawner, ctx context.Context) error {
	c := luaCallback{
		parent:       parent,
		state:        l,
		onPacket:     nil,
		filterPacket: nil,
		rcv:          nil,
		rcvCtx:       context.Background(), // Just some bullshit incase we dont use this.
		luaCancel:    nil,
		luaCtx:       nil,
	}
	c.luaCtx, c.luaCancel = context.WithCancel(ctx)
	defer c.luaCancel()
	filterPacket := l.GetGlobal("EzpFilter")
	if v, ok := filterPacket.(*lua.LFunction); ok {
		c.filterPacket = v
		err := sp.TrySetFilterCallback(c.handleFilters, c.luaCtx)
		if err != nil {
			return err
		}
	}
	onPacket := l.GetGlobal("EzpOnPacket")
	if v, ok := onPacket.(*lua.LFunction); ok {
		if c.filterPacket != nil {
			return errors.New("EzpFilter and EzpOnPacket cannot be defined together")
		}
		c.onPacket = v
		c.rcv, c.rcvCtx, c.luaCancel = sp.GetRecvChan(c.luaCtx)
	}
	if c.onPacket == nil && filterPacket == nil {
		return errors.New("no callbacks")
	}
	c.run()
	return nil
}
