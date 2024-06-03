package proxy

/*
Udp over TCP isn't fully setup yet, for now we just use it to literally send UDP over TCP with one given addresses, when a TCP client is
connected the listener will wait for the tcp client to close before continuing data.
*/

import (
	"context"
	"errors"
	"ezproxy/handler"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"
)

const UdpOverTcpMpxName string = "UdpOverTcp"

type UdpOverTcpProxy struct {
	ctx        context.Context                // Proxy context
	ctxCancel  context.CancelCauseFunc        // Cancel context
	client     *net.TCPConn                   // Client connection (TCP)
	serverAddr *net.UDPAddr                   // Server address (UDP)
	proxyUdp   *net.UDPConn                   // Proxy
	pktChan    chan<- handler.ProxyPacketData // Packet channel
	logger     *slog.Logger
}

// Client is TCP
func (t *UdpOverTcpProxy) listenClient() {
	for t.ctx.Err() == nil {
		buffer := make([]byte, 4096)
		t.client.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := t.client.Read(buffer)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			// Terminated
			if err == io.EOF {
				t.logger.Debug("Connection closed")
				t.ctxCancel(handler.ErrProxyRetry)
			} else {
				t.logger.Debug("Closing due to error", "Error", err.Error())
				t.ctxCancel(fmt.Errorf("failed to read from proxy: %v", err))
			}
			return
		}
		t.logger.Debug("Sending packet data", "Serverbound", true, "Source", t.client.RemoteAddr(), "Data", buffer[:n])
		t.pktChan <- handler.ProxyPacketData{
			Serverbound: true,
			Source:      t.client.RemoteAddr(),
			Dest:        t.serverAddr,
			Data:        buffer[:n],
		}
	}
}

// Server is UDP
func (t *UdpOverTcpProxy) listenServer() {
	for t.ctx.Err() == nil {
		buffer := make([]byte, 4096)
		t.proxyUdp.SetReadDeadline(time.Now().Add(time.Second * 2))
		n, from, err := t.proxyUdp.ReadFromUDP(buffer)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			t.logger.Debug("Closing due to error", "Error", err.Error())
			t.ctxCancel(err)
			return
		}
		pktData := handler.ProxyPacketData{
			Data:        buffer[:n],
			Source:      from,
			Dest:        t.client.RemoteAddr(),
			Serverbound: false,
		}
		if !compareNetAddr(from, t.serverAddr) {
			// Unknown sender, ignore packet
			continue
		}
		t.logger.Debug("Sending packet data", "Serverbound", false, "Source", pktData.Source, "Data", pktData.Data)
		t.pktChan <- pktData
	}
}

func (t *UdpOverTcpProxy) MpxName() string {
	return UdpOverTcpMpxName
}

func (t *UdpOverTcpProxy) Network() string {
	// Well, its sorta both, but we'll say TCP
	// return "tcp>udp"
	return "tcp"
}

func (t *UdpOverTcpProxy) GetClientAddr() net.Addr {
	return t.client.RemoteAddr()
}

// The client is TCP
func (t *UdpOverTcpProxy) SendToClient(data []byte) error {
	_, err := t.client.Write(data)
	if err != nil {
		t.logger.Debug("Failed to send data to client", "Data", data, "Error", err.Error())
	} else {
		t.logger.Debug("Sent data to client", "Data", data)
	}
	return err
}

// The server is UDP
func (t *UdpOverTcpProxy) SendToServer(data []byte) error {
	_, err := t.proxyUdp.Write(data)
	if err != nil {
		t.logger.Debug("Failed to send data to server", "Data", data, "Error", err.Error())
	} else {
		t.logger.Debug("Sent data to server", "Data", data)
	}
	return err
}

func (t *UdpOverTcpProxy) Init(pktChan chan<- handler.ProxyPacketData, ctx context.Context, cancel context.CancelCauseFunc) error {
	if t.pktChan != nil {
		t.logger.Error("Already initialized")
		return errors.New("already initialized")
	}
	t.logger.Debug("Initializing")
	t.pktChan = pktChan
	t.ctx = ctx
	t.ctxCancel = cancel
	go t.listenClient()
	go t.listenServer()
	return nil
}

func newUdpOverTcpProxy(client *net.TCPConn, proxy *net.UDPConn, server *net.UDPAddr) handler.IProxy {
	udot := &UdpOverTcpProxy{
		client:     client,
		serverAddr: server,
		proxyUdp:   proxy,
		logger:     slog.Default(),
	}
	return udot
}

// Listen on TCP, wait for connections
// When we get one create a new UDP connection to server (proxyCon)
func UdpOverTcpListener(ctx context.Context, cancel context.CancelCauseFunc, ps handler.IConnectionAdder) {
	logger := slog.Default()
	addr, err := ps.GetProxyAddr(UdpOverTcpMpxName)
	if err != nil {
		panic(fmt.Sprintf("Failed to get proxy address for Mpx: %v", err))
	}
	pAddr, err := net.ResolveTCPAddr("tcp", addr.String())
	if err != nil {
		logger.Warn("Failed to resolve TCP ProxyAddr", "ProxyAddr", addr.String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve tcp proxy address: %v", err))
		return
	}
	sAddr, err := net.ResolveUDPAddr("udp", ps.GetServerAddr().String())
	if err != nil {
		logger.Warn("Failed to resolve ServerAddr", "ServerAddr", ps.GetServerAddr().String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve tcp server address: %v", err))
		return
	}
	listenCon, err := net.ListenTCP("tcp", pAddr)
	if err != nil {
		// Can't open UDP connection, fatal error.
		logger.Warn("Failed to listen on proxy", "Error", err.Error(), "ProxyAddress", pAddr.String())
		cancel(fmt.Errorf("failed to listen on tcp proxy: %v", err))
		return
	}
	logger.Debug("Listener started", "MpxName", UdpOverTcpMpxName)
	conId := -1
	for ctx.Err() == nil {
		if conId != -1 {
			// Check if the connection is still alive
			c, err := ps.GetProxy(conId)
			if err != nil || !c.IsAlive() {
				// Reset the id - The connection should be dead.
				conId = -1
			}
			continue
		}
		// Get a new connection
		listenCon.SetDeadline(time.Now().Add(time.Second * 2))
		c, err := listenCon.AcceptTCP()
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			logger.Debug("Failed to accept TCP connection", "Error", err.Error())
			cancel(fmt.Errorf("failed to accept tcp connection: %v", err))
			continue
		}
		if ctx.Err() != nil {
			logger.Debug("Unsticking connection")
			// Self connect to unstick connection
			break
		}
		// Create new connection to server
		pCon, err := net.DialUDP("udp", nil, sAddr)
		if err != nil {
			logger.Warn("Failed to create new connection to server", "Error", err.Error(), "ServerAddress", sAddr.String())
			c.Close()
			cancel(fmt.Errorf("failed to create new connection to server: %v", err))
			continue
		}
		ps.AddConnection(newUdpOverTcpProxy(c, pCon, sAddr))
	}
}
