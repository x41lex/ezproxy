package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Container for IProxy
type ProxyContainer struct {
	spawner         IProxySpawner
	ctx             context.Context
	ctxCancel       context.CancelCauseFunc
	px              IProxy
	pktChan         <-chan ProxyPacketData
	id              int
	statsLock       sync.RWMutex
	bytesSent       uint64
	lastContactTime time.Time
	logger          *slog.Logger
}

// Handles packets recived
func (pc *ProxyContainer) handlePacket() {
	for {
		select {
		case <-pc.ctx.Done():
			return
		case data := <-pc.pktChan:
			// Setup the flags
			flags := CapFlags(0)
			if data.Serverbound {
				flags |= CapFlag_ToServer
			}
			// Only send it if the callback says we can
			if pc.spawner.HandleSend(data.Data, flags, pc) {
				var err error
				pc.logger.Debug("Forwarding packet", "Source", data.Source, "Dest", data.Dest, "Serverbound", data.Serverbound, "Data", data.Data, "Flags", flags)
				if data.Serverbound {
					// From client to server
					err = pc.px.SendToServer(data.Data)
				} else {
					// From server to client
					err = pc.px.SendToClient(data.Data)
				}
				if err != nil {
					// Add error if needed
					pc.logger.Debug("Error in sending packet", "Error", err.Error())
					pc.spawner.HandleError(err, pc)
				} else {
					pc.statsLock.Lock()
					pc.lastContactTime = time.Now()
					pc.bytesSent += uint64(len(data.Data))
					pc.statsLock.Unlock()
				}
			} else {
				pc.logger.Debug("Filtering packet", "Source", data.Source, "Dest", data.Dest, "Serverbound", data.Serverbound, "Data", data.Data, "Flags", flags)
			}
		}
	}
}

// Inject data to client, calls the sendCallback and doesn't send it if the callback returns false
func (pc *ProxyContainer) SendToClient(data []byte) error {
	if !pc.spawner.HandleSend(data, CapFlag_Injected, pc) {
		pc.logger.Debug("Not sending packet from SendToClient", "Data", data, "Dest", pc.GetClientAddr(), "Serverbound", false)
		// Dont send
		return nil
	}
	err := pc.px.SendToClient(data)
	if err != nil {
		pc.logger.Debug("Error sending data to client", "Error", err.Error())
		pc.spawner.HandleError(err, pc)
		return err
	}
	pc.statsLock.Lock()
	pc.lastContactTime = time.Now()
	pc.bytesSent += uint64(len(data))
	pc.statsLock.Unlock()
	return nil
}

// Inject data to server, calls the sendCallback and doesn't send it if the callback returns false
func (pc *ProxyContainer) SendToServer(data []byte) error {
	if !pc.spawner.HandleSend(data, CapFlag_ToServer|CapFlag_Injected, pc) {
		pc.logger.Debug("Not sending packet from SendToServer", "Data", data, "Dest", pc.GetServerAddr(), "Serverbound", true)
		// Dont send
		return nil
	}
	err := pc.px.SendToServer(data)
	if err != nil {
		pc.logger.Debug("Error sending data to client", "Error", err.Error())
		pc.spawner.HandleError(err, pc)
		return err
	}
	pc.statsLock.Lock()
	pc.lastContactTime = time.Now()
	pc.bytesSent += uint64(len(data))
	pc.statsLock.Unlock()
	return nil
}

// Get server address
func (pc *ProxyContainer) GetServerAddr() net.Addr {
	return pc.spawner.GetServerAddr()
}

// Get client address
func (pc *ProxyContainer) GetClientAddr() net.Addr {
	return pc.px.GetClientAddr()
}

// Closes the proxy container
func (pc *ProxyContainer) Close() error {
	pc.logger.Debug("Closing ProxyContainer")
	pc.ctxCancel(ErrProxyClosedOk)
	return nil
}

func (pc *ProxyContainer) Network() string {
	return pc.px.Network()
}

// Checks if the proxy is still alive
func (pc *ProxyContainer) IsAlive() bool {
	return pc.ctx.Err() == nil
}

func (pc *ProxyContainer) GetId() int {
	return pc.id
}

func (pc *ProxyContainer) Cancel(cause error) {
	pc.ctxCancel(cause)
}

func (pc *ProxyContainer) GetBytesSent() uint64 {
	pc.statsLock.RLock()
	defer pc.statsLock.RUnlock()
	return pc.bytesSent
}

func (pc *ProxyContainer) LastContactTimeAgo() time.Duration {
	pc.statsLock.RLock()
	defer pc.statsLock.RUnlock()
	return time.Since(pc.lastContactTime)
}

// Creates a new container
func NewProxyContainer(parent IProxySpawner, px IProxy, id int) (IProxyContainer, error) {
	pktChan := make(chan ProxyPacketData)
	pCtx, pCtxCancel := context.WithCancelCause(parent.GetContext())
	pc := &ProxyContainer{
		spawner:         parent,
		pktChan:         pktChan,
		px:              px,
		ctx:             pCtx,
		ctxCancel:       pCtxCancel,
		id:              id,
		logger:          slog.Default(),
		statsLock:       sync.RWMutex{},
		bytesSent:       0,
		lastContactTime: time.Now(),
	}
	go pc.handlePacket()
	pc.logger.Debug("Init on new IProxy", "Id", id, "Client", px.GetClientAddr())
	err := px.Init(pktChan, pCtx, pCtxCancel)
	if err != nil {
		pCtxCancel(fmt.Errorf("failed to initilize: %v", err))
		return nil, err
	}
	return pc, nil
}
