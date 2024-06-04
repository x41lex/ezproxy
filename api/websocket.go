package api

import (
	"context"
	"encoding/json"
	"errors"
	"ezproxy/handler"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

type wsServerTypes int
type wsFilterValue int

const (
	wsServerError  wsServerTypes = -1 // Should never be received, used as a internal nil
	wsServerPacket wsServerTypes = 1  // A packet

	wsFilterDrop  wsFilterValue = -1 // Wait for a action
	wsFilterWait  wsFilterValue = 0  // Drop the packet
	wsFilterAllow wsFilterValue = 1  // Allow the packet
)

// WebSocket connection
type wsApi struct {
	parent        *WebApi
	ws            *websocket.Conn
	ctx           context.Context
	cancel        context.CancelFunc
	canInject     bool
	canFilter     bool
	canClose      bool
	filterMap     map[int]wsFilterValue
	filterChan    chan struct{}
	pktIdLock     sync.Mutex
	pktId         int
	defaultAction wsFilterValue
	filterTimeout time.Duration
	networkFilter string // "" = No filter

	recvChan   <-chan handler.PacketChanData
	recvCancel context.CancelFunc
}

type wsPacket struct {
	PktNum  int              // The index of the packet on the WebSocket - Only used for filtering right now
	ProxyId int              // ID of the proxy that this was sent over
	Network string           // Network this packet was sent on, 'tcp' or 'udp'
	Source  string           // Source of this packet
	Dest    string           // Destination of this packet
	Data    []byte           // Packet data
	Flags   handler.CapFlags // Flags, any CapFlag_*, if CapFlag_Inject is set this packet cannot be filtered.
}

type wsServerMsg struct {
	Type wsServerTypes
	Data any
}

// !! DON'T USE !!
// Breaking changes may happen in the future - Only use this if you really know what your doing.
func (w *wsApi) _sendRaw(status int, wsType wsServerTypes, data any) error {
	msg := baseResponse{
		Status: status,
	}
	if status != 200 {
		// I *really* hope thats a string, but I don't think slowing everything down
		// to check using reflection is worth it.
		msg.Data = data
	} else {
		if wsType == wsServerError {
			// Programming error, cannot be resolved.
			w.Close()
			panic("attempted to send a wsServerError")
		}
		msg.Data = wsServerMsg{
			Type: wsType,
			Data: data,
		}
	}
	eData, err := json.Marshal(msg)
	if err != nil {
		// Fatal as this should only be called by internal methods
		// if it failed to marshal its because something is seriously fucked
		// and the problem will repeat.
		panic(err.Error())
	}
	err = w.ws.Write(w.ctx, websocket.MessageText, eData)
	if err != nil {
		// Non fatal, the WebSocket is likely closed.
		w.parent.logger.Warn("Failed to send data on WebSocket", "Error", err.Error())
		return err
	}
	return nil
}

// Sends a wsPacket over the websocket
func (w *wsApi) sendPacket(pkt *wsPacket) error {
	w.parent.logger.Debug("Sending packet to WebSocket", "PktNum", pkt.PktNum, "ProxyId", pkt.ProxyId, "Data", pkt.Data, "Source", pkt.Source, "Dest", pkt.Dest, "Network", pkt.Network, "Flags", pkt.Flags)
	return w._sendRaw(200, wsServerPacket, pkt)
}

// Sends a error on the websocket
func (w *wsApi) sendError(status int, msg string) error {
	w.parent.logger.Debug("Sending error", "Status", status, "Message", msg)
	return w._sendRaw(status, wsServerError, msg)
}

// Closes the web socket
func (w *wsApi) Close() error {
	w.cancel()
	if w.recvCancel != nil {
		w.recvCancel()
	}
	return nil
}

func (w *wsApi) recv() {
	for {
		select {
		case pkt := <-w.recvChan:
			// This should only ever be a open channel if the callback wasn't set.
			px, err := w.parent.handler.GetProxy(pkt.ProxyId)
			if err != nil {
				w.parent.logger.Warn("Got packet from recvChan with proxy ID that lead to a non existent proxy", "Id", pkt.ProxyId)
				continue
			}
			w.handleRecv(pkt.Data, pkt.Flags, px)
		case <-w.ctx.Done():
			return
		}
	}
}

// listen for data, stops when the context is done
func (w *wsApi) listen() error {
	for {
		select {
		case <-w.ctx.Done():
			// Context is done, we can close.
			w.Close()
			return nil
		default:
			// This can block forever, the read function will also stop when the context dies
			t, data, err := w.ws.Read(w.ctx)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					// We don't care about timeouts
					continue
				} else if errors.Is(err, context.Canceled) {
					w.parent.logger.Debug("Context was canceled, closing WebSocket")
					// Not a error
					w.Close()
					return nil
				}
				// Log a error
				w.parent.logger.Warn("WebSocket read failed", "Error", err)
				w.Close()
				return err
			}
			w.handleClientAction(t, data)
		}
	}
}

func (w *wsApi) matchesNetworkFilter(p handler.IProxyContainer) bool {
	if w.networkFilter == "" {
		return true
	}
	return w.networkFilter == p.Network()
}

// id is -1 if addId is false
func (w *wsApi) sendPkt(addId bool, data []byte, flags handler.CapFlags, p handler.IProxyContainer) (pkt *wsPacket, err error) {
	pkt = &wsPacket{
		PktNum:  -1,
		ProxyId: p.GetId(),
		Network: p.Network(),
		Data:    data,
		Flags:   flags,
	}
	if flags.IsServerbound() {
		// Client => Server
		pkt.Source = p.GetClientAddr().String()
		pkt.Dest = p.GetServerAddr().String()
	} else {
		// Server => Client
		pkt.Source = p.GetServerAddr().String()
		pkt.Dest = p.GetClientAddr().String()
	}
	if addId {
		// Get packet ID
		w.pktIdLock.Lock()
		pkt.PktNum = w.pktId
		// Wait for a change
		w.pktId++
		w.pktIdLock.Unlock()
		// We don't filter injected packets
		if w.canFilter && !flags.IsInjected() {
			w.parent.logger.Debug("Added packet to filterMap", "PktNum", pkt.PktNum, "Source", pkt.Source, "Dest", pkt.Dest, "Data", pkt.Data, "Network", pkt.Network, "Flags", pkt.Flags)
			w.filterMap[pkt.PktNum] = wsFilterWait
		}
	}
	return pkt, w.sendPacket(pkt)
}

func (w *wsApi) handleRecv(data []byte, flags handler.CapFlags, p handler.IProxyContainer) {
	if !w.matchesNetworkFilter(p) {
		return
	}
	w.sendPkt(false, data, flags, p)
}

func (w *wsApi) handleRecvFilter(data []byte, flags handler.CapFlags, p handler.IProxyContainer) (shouldSend bool) {
	if !w.matchesNetworkFilter(p) {
		return w.defaultAction == wsFilterAllow
	}
	pkt, err := w.sendPkt(true, data, flags, p)
	if err != nil {
		w.parent.logger.Warn("failed to send packet")
		return w.defaultAction == wsFilterAllow
	}
	// Injected packets are always sent.
	if pkt.Flags.IsInjected() {
		return true
	}
	timer := time.NewTimer(w.filterTimeout)
	for {
		select {
		case <-w.ctx.Done():
			return w.defaultAction == wsFilterAllow
		case <-timer.C:
			// Default action
			w.parent.logger.Warn("Packet filtering timed out", "Source", pkt.Source, "Dest", pkt.Dest, "Flags", pkt.Flags, "PktNum", pkt.PktNum, "DefaultAction", w.defaultAction)
			w.sendError(http.StatusRequestTimeout, "packet timed out")
			return w.defaultAction == wsFilterAllow
		case <-w.filterChan:
			// Something in the map was updated
			switch w.filterMap[pkt.PktNum] {
			case wsFilterAllow:
				w.parent.logger.Debug("Filtering packet", "Target", pkt.PktNum, "Action", "allow")
				return true
			case wsFilterDrop:
				w.parent.logger.Debug("Filtering packet", "Target", pkt.PktNum, "Action", "drop")
				return false
			default:
				continue
			}
		}
	}
}

func (w *wsApi) handleSend(data []byte, flags handler.CapFlags, p handler.IProxyContainer) (shouldSend bool) {
	if w.canFilter {
		return w.handleRecvFilter(data, flags, p)
	}
	w.handleRecv(data, flags, p)
	return true
}

func (a *WebApi) newWebSocket(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(a.ctx)
	ws := &wsApi{
		parent:        a,
		ws:            nil,
		canInject:     false,
		canFilter:     false,
		canClose:      false,
		defaultAction: wsFilterAllow,
		networkFilter: "",
		filterChan:    make(chan struct{}),
		filterMap:     make(map[int]wsFilterValue),
		pktIdLock:     sync.Mutex{},
		pktId:         0,
		filterTimeout: time.Second * 2, // This will probably be a config option in the future.
		cancel:        cancel,
		ctx:           ctx,
		recvChan:      nil,
		recvCancel:    nil,
	}
	a.logger.Debug("Creating new WebSocket")
	defer ws.Close()
	// Handle permissions first - if we don't use auth we assume all
	key := uint64(0)
	val := int(AuthAll)
	if a.auth != nil {
		var ok bool
		key, val, ok = a.auth.getAuthValues(w, r)
		if !ok {
			a.logger.Error("authentication of both succeeded & failed", "Key", key, "Map", a.auth.keys)
			// I don't understand how this is possible.
			return
		}
	}
	// To request permissions we go add it to the query
	// 'close'
	qr := r.URL.Query()
	if qr.Has("close") {
		if checkPermission(val, AuthCanClose) {
			a.logger.Debug("WebSocket can close")
			ws.canClose = true
		} else {
			a.logger.Debug("Missing permissions for 'close' websocket not ok")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("missing permissions for 'close'"))
			return
		}
	}
	if qr.Has("inject") {
		if checkPermission(val, AuthCanInject) {
			ws.canInject = true
			a.logger.Debug("WebSocket can inject")
		} else {
			a.logger.Debug("Missing permissions for 'inject' websocket not ok")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("missing permissions for 'inject'"))
			return
		}
	}
	if qr.Has("filter") {
		if !checkPermission(val, AuthCanFilter) {
			a.logger.Debug("Missing permissions for 'filter' websocket not ok")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("missing permissions for 'filter'"))
			return
		}
		ws.canFilter = true
		a.logger.Debug("WebSocket can filter")
		// TODO: Figure out how to drop this when something goes wrong I.E If the websocket dies.
		if qr.Has("default") {
			def := qr.Get("default")
			switch def {
			case "drop":
				a.logger.Debug("Default action 'drop'")
				ws.defaultAction = wsFilterDrop
			case "allow":
				a.logger.Debug("Default action 'allow'")
				ws.defaultAction = wsFilterAllow
			default:
				a.logger.Debug("Invalid default action", "Action", def)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("'default' value is invalid, must be 'drop' or 'allow"))
				return
			}
		} else {
			a.logger.Debug("No default action, set to 'allow'")
		}
	}
	// If we can filter we need to need to set the filter callback, if we don't need to filter we can just get a recvChan
	// if the SendCallback is already set we can just ignore it.
	if ws.canFilter {
		err := a.handler.TrySetFilterCallback(ws.handleSend, ws.ctx)
		if err != nil {
			a.logger.Info("Attempted to set the sendFilter callback failed", "Error", err.Error())
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("Filterer already exists"))
			return
		}
	} else {
		// I don't think we need this channel.
		x, _, z := a.handler.GetRecvChan(a.ctx)
		ws.recvCancel = z
		ws.recvChan = x
	}
	ws.networkFilter = qr.Get("network")
	if ws.networkFilter != "" && ws.networkFilter != "tcp" && ws.networkFilter != "udp" {
		a.logger.Debug("Invalid network filter", "Filter", ws.networkFilter)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("network must be 'tcp', 'udp' or left empty"))
		return
	}
	// Start the websocket
	var err error
	ws.ws, err = websocket.Accept(w, r, nil)
	if err != nil {
		a.logger.Warn("Failed to accept websocket request", "Err", err.Error())
		return
	}
	if ws.recvChan != nil {
		go ws.recv()
	}
	ws.listen()
	a.logger.Debug("WebSocket closing, removing callbacks and filter")
}
