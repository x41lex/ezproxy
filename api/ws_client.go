package api

import (
	"encoding/json"
	"ezproxy/handler"
	"fmt"
	"net/http"

	"nhooyr.io/websocket"
)

type wsReqType int

const (
	wsReqInject wsReqType = 1
	wsReqClose  wsReqType = 2
	wsReqFilter wsReqType = 3

	wsTargetAll int = -1 // Target all proxies

	wsInjToClient uint64 = 1 << 0 // Client bit
	wsInjToServer uint64 = 1 << 1 // Server bit

	wsFilterShouldSend uint64 = 1 << 0 // Send/Drop bit (Clear: Drop)
)

type wsClientMsg struct {
	Type   wsReqType
	Target int    // Inject, Close: Target proxy, Filter: Target proxy
	Data   []byte // Inject: Inject data
	Extra  uint64 // Inject: 0, Send to Client. 1, Send to Server. Filter: 0, Drop/Send (0/1)
}

func (w *wsApi) handleClientAction(t websocket.MessageType, data []byte) {
	// Only accept text
	if t != websocket.MessageText {
		w.sendError(http.StatusBadRequest, "Data type must be Text JSON")
		return
	}
	// Decode (Should be JSON of a WsClientMsgBase)
	msg := wsClientMsg{}
	err := json.Unmarshal(data, &msg)
	if err != nil {
		w.parent.logger.Debug("Got bad JSON data")
		w.sendError(http.StatusBadRequest, "Bad JSON data")
		return
	}
	switch msg.Type {
	case wsReqInject:
		if !w.canInject {
			w.sendError(http.StatusForbidden, "Missing permissions to inject")
			return
		}
		// Check what we are ok sending to
		toServer := msg.Extra&wsInjToServer != 0
		toClient := msg.Extra&wsInjToClient != 0
		if !toServer && !toClient {
			w.sendError(http.StatusBadRequest, "toClient and/or toServer must be set in 'Extra'")
			return
		}
		if msg.Target == wsTargetAll {
			// Send to everyone
			if toServer {
				w.parent.handler.SendToAllServers(msg.Data)
			}
			if toClient {
				w.parent.handler.SendToAllClients(msg.Data)
			}
			return
		}
		// Send to a target proxy
		px, err := w.parent.handler.GetProxy(msg.Target)
		if err != nil {
			w.parent.logger.Debug("Proxy not found", "Id", msg.Target)
			w.sendError(http.StatusNotFound, fmt.Sprintf("proxy not found: %v", err))
			return
		}
		w.parent.logger.Debug("Sending to data to target", "Id", msg.Target, "Data", msg.Data, "ToServer", toServer, "ToClient", toClient)
		if toServer {
			px.SendToServer(msg.Data)
		}
		if toClient {
			px.SendToClient(msg.Data)
		}
		return
	case wsReqClose:
		if !w.canClose {
			w.sendError(http.StatusForbidden, "Missing permissions to close")
			return
		}
		if msg.Target == wsTargetAll {
			w.parent.logger.Debug("Closing all proxies")
			for _, v := range w.parent.handler.GetAllProxies() {
				v.Cancel(handler.ErrProxyClosedOk)
			}
			return
		}
		px, err := w.parent.handler.GetProxy(msg.Target)
		if err != nil {
			w.sendError(http.StatusNotFound, fmt.Sprintf("proxy not found: %v", err))
			return
		}
		w.parent.logger.Debug("Closing all proxy", "Target", msg.Target)
		px.Cancel(handler.ErrProxyClosedOk)
		return
	case wsReqFilter:
		if !w.canFilter {
			w.sendError(http.StatusForbidden, "Missing permissions to inject")
			return
		}
		if _, found := w.filterMap[msg.Target]; !found {
			w.sendError(http.StatusNotFound, "packet not found")
			return
		}
		if w.filterMap[msg.Target] != wsFilterWait {
			w.sendError(http.StatusGone, "packet already handled")
			return
		}
		flag := msg.Extra & wsFilterShouldSend
		if flag == 0 {
			w.filterMap[msg.Target] = wsFilterDrop
			w.parent.logger.Debug("Setting filter map", "Target", msg.Target, "Action", "drop")
		} else {
			w.parent.logger.Debug("Setting filter map", "Target", msg.Target, "Action", "allow")
			w.filterMap[msg.Target] = wsFilterAllow
		}
		w.filterChan <- struct{}{}
	default:
		w.sendError(http.StatusBadRequest, "Unknown Type")
		return
	}
}
