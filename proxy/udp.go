package proxy

import (
	"context"
	"errors"
	"ezproxy/handler"
	"fmt"
	"log/slog"
	"net"
	"time"
)

type UdpProxy struct {
	ctx          context.Context
	ctxCancel    context.CancelCauseFunc
	client       *net.UDPAddr
	server       *net.UDPAddr
	proxy        *net.UDPConn
	pktChan      chan<- handler.ProxyPacketData
	firstPktData []byte
	logger       *slog.Logger
}

// Listener for UDP proxies
func (u *UdpProxy) listen() {
	u.pktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      u.client,
		Dest:        u.server,
		Data:        u.firstPktData,
	}
	u.firstPktData = nil
	for u.ctx.Err() == nil {
		buffer := make([]byte, 4096)
		u.proxy.SetReadDeadline(time.Now().Add(time.Second * 2))
		n, from, err := u.proxy.ReadFromUDP(buffer)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			u.logger.Debug("Closing due to error", "Error", err.Error())
			u.ctxCancel(err)
			return
		}
		pktData := handler.ProxyPacketData{
			Data:   buffer[:n],
			Source: from,
		}
		if compareNetAddr(from, u.client) {
			// Serverbound
			pktData.Serverbound = true
			pktData.Dest = u.server
		} else if compareNetAddr(from, u.server) {
			// Clientbound
			pktData.Serverbound = false
			pktData.Dest = u.client
		} else {
			// Unknown sender, ignore packet
			continue
		}
		u.logger.Debug("Sending packet data", "Serverbound", pktData.Serverbound, "Source", pktData.Source, "Dest", pktData.Dest, "Data", pktData.Data)
		u.pktChan <- pktData
	}
}

func (u *UdpProxy) Network() string {
	return "udp"
}

// Initialize UDP proxy
func (u *UdpProxy) Init(pktChan chan<- handler.ProxyPacketData, ctx context.Context, cancel context.CancelCauseFunc) error {
	if u.pktChan != nil {
		u.logger.Error("Already initialized")
		return errors.New("already initialized")
	}
	u.logger.Debug("Initializing")
	u.pktChan = pktChan
	u.ctx = ctx
	u.ctxCancel = cancel
	// Send the first packet we got to the server
	go u.listen()
	return nil
}

// Gets client address
func (u *UdpProxy) GetClientAddr() net.Addr {
	return u.client
}

// Send packet to client
func (u *UdpProxy) SendToClient(data []byte) error {
	_, err := u.proxy.WriteToUDP(data, u.client)
	if err != nil {
		u.logger.Debug("Failed to send data to client", "Data", data, "Error", err.Error())
	} else {
		u.logger.Debug("Sent data to client", "Data", data)
	}
	return err
}

// Send packet to server
func (u *UdpProxy) SendToServer(data []byte) error {
	_, err := u.proxy.WriteToUDP(data, u.server)
	if err != nil {
		u.logger.Debug("Failed to send data to server", "Data", data, "Error", err.Error())
	} else {
		u.logger.Debug("Sent data to server", "Data", data)
	}
	return err
}

// Create a new UDP proxy
func newUdpProxy(client *net.UDPAddr, proxy *net.UDPConn, server *net.UDPAddr, firstPkt []byte) handler.IProxy {
	// These should convert properly always because we pass them from Handler
	up := &UdpProxy{
		client:       client,
		server:       server,
		proxy:        proxy,
		firstPktData: firstPkt,
		logger:       slog.Default(),
	}
	return up
}

// Listener for new UDP proxies
func UdpListener(ctx context.Context, cancel context.CancelCauseFunc, ps handler.IConnectionAdder) {
	logger := slog.Default()
	// Get the addresses in UDP form
	pAddr, err := net.ResolveUDPAddr("udp", ps.GetProxyAddr().String())
	if err != nil {
		logger.Warn("Failed to resolve ProxyAddr", "ProxyAddr", ps.GetProxyAddr().String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve udp proxy address: %v", err))
		return
	}
	sAddr, err := net.ResolveUDPAddr("udp", ps.GetServerAddr().String())
	if err != nil {
		logger.Warn("Failed to resolve ServerAddr", "ServerAddr", ps.GetServerAddr().String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve udp server address: %v", err))
		return
	}
	// Open a UDP listener
	id := -1
	pCon, err := net.ListenUDP("udp", pAddr)
	if err != nil {
		// Can't open UDP connection, fatal error.
		logger.Warn("Failed to listen on proxy", "Error", err.Error(), "ProxyAddress", pAddr.String())
		cancel(fmt.Errorf("failed to listen on udp proxy: %v", err))
		return
	}
	logger.Debug("Listener started")
	for ctx.Err() == nil {
		// If we have a proxy already
		if id != -1 {
			// Check if its been pruned
			cl, err := ps.GetProxy(id)
			if err != nil || !cl.IsAlive() {
				// If it has, close pCon (Incase the proxy didn't, but it probably did.)
				pCon.Close()
				// Create a new pCon, because the proxy probably closed it.
				pCon, err = net.ListenUDP("udp", pAddr)
				if err != nil {
					// Can't open UDP connection, fatal error.
					logger.Warn("Failed to re listen on proxy address", "ProxyAddress", pAddr.String(), "Error", err.Error())
					cancel(fmt.Errorf("failed to re listen on udp proxy: %v", err))
					return
				}
				// Set the id to unused.
				id = -1
			} else {
				// Wait for a bit so we don't just spam check it for no reason.
				// I don't really know how long this wait should be, I think this is good though?
				time.Sleep(time.Second * 1)
				continue
			}
		}
		// Wait for traffic on proxy
		buffer := make([]byte, 4096)
		// Set timeout so we check h.alive every once and a while.
		pCon.SetReadDeadline(time.Now().Add(time.Second * 2))
		n, from, err := pCon.ReadFromUDP(buffer)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			logger.Debug("Failed to read from proxy", "Error", err.Error())
			continue
		}
		// From server, ignored.
		if compareNetAddr(sAddr, from) {
			logger.Debug("Ignoring data from server in listener", "ServerAddress", sAddr.String(), "From", from.String())
			continue
		}
		// New client - The client is nil as this is UDP
		pc, err := ps.AddConnection(newUdpProxy(from, pCon, sAddr, buffer[:n]))
		if err != nil {
			logger.Debug("Failed to add new connection", "Error", err.Error(), "ServerAddress", sAddr.String(), "From", from.String())
			id = -1
			continue
		}
		id = pc.GetId()
		logger.Debug("Added new connection", "ServerAddress", sAddr.String(), "From", from.String(), "Id", id)
	}
}
