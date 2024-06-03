package proxy

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

const tcpMpxName string = "TcpPlain"

// TCP proxy
type TcpProxy struct {
	ctx       context.Context                // Proxy context
	ctxCancel context.CancelCauseFunc        // Cancel context
	client    *net.TCPConn                   // Client connection
	server    *net.TCPConn                   // Server connection
	pktChan   chan<- handler.ProxyPacketData // Packet channel
	logger    *slog.Logger
}

// Listen for packets
func (t *TcpProxy) listen(c *net.TCPConn) {
	// Serverbound
	serverbound := true
	source := t.client
	dest := t.server
	if compareNetAddr(c.RemoteAddr(), t.server.RemoteAddr()) {
		// Clientbound
		source = t.server
		dest = t.client
		serverbound = false
	}
	for t.ctx.Err() == nil {
		buffer := make([]byte, 4096)
		c.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := c.Read(buffer)
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
		t.logger.Debug("Sending packet data", "Serverbound", serverbound, "Source", source.RemoteAddr(), "Dest", dest.RemoteAddr(), "Data", buffer[:n])
		t.pktChan <- handler.ProxyPacketData{
			Serverbound: serverbound,
			Source:      source.RemoteAddr(),
			Dest:        dest.RemoteAddr(),
			Data:        buffer[:n],
		}
	}
}

func (t *TcpProxy) MpxName() string {
	return tcpMpxName
}

func (t *TcpProxy) Network() string {
	return "tcp"
}

func (t *TcpProxy) GetClientAddr() net.Addr {
	return t.client.RemoteAddr()
}

// Send data to client
func (t *TcpProxy) SendToClient(data []byte) error {
	_, err := t.client.Write(data)
	if err != nil {
		t.logger.Debug("Failed to send data to client", "Data", data, "Error", err.Error())
	} else {
		t.logger.Debug("Sent data to client", "Data", data)
	}
	return err
}

// Send data to server
func (t *TcpProxy) SendToServer(data []byte) error {
	_, err := t.server.Write(data)
	if err != nil {
		t.logger.Debug("Failed to send data to server", "Data", data, "Error", err.Error())
	} else {
		t.logger.Debug("Sent data to server", "Data", data)
	}
	return err
}

// Initialize the proxy
func (t *TcpProxy) Init(pktChan chan<- handler.ProxyPacketData, ctx context.Context, cancel context.CancelCauseFunc) error {
	if t.pktChan != nil {
		t.logger.Error("Already initialized")
		return errors.New("already initialized")
	}
	t.logger.Debug("Initializing")
	t.pktChan = pktChan
	t.ctx = ctx
	t.ctxCancel = cancel
	go t.listen(t.client)
	go t.listen(t.server)
	return nil
}

// Create a new TcpProxy
func newTcpProxy(client net.Conn, server net.Conn) handler.IProxy {
	t := &TcpProxy{
		client: client.(*net.TCPConn),
		server: server.(*net.TCPConn),
		logger: slog.Default(),
	}
	return t
}

// Listen & Accept new connections to create new proxies
func TcpListener(ctx context.Context, cancel context.CancelCauseFunc, ps handler.IConnectionAdder) {
	logger := slog.Default()
	// Convert to TCP form
	addr, err := ps.GetProxyAddr(tcpMpxName)
	if err != nil {
		panic(fmt.Sprintf("Failed to get proxy address for Mpx: %v", err))
	}
	pAddr, err := net.ResolveTCPAddr("tcp", addr.String())
	if err != nil {
		logger.Warn("Failed to resolve ProxyAddr", "ProxyAddr", addr.String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve proxy addr: %v", err))
		return
	}
	sAddr, err := net.ResolveTCPAddr("tcp", ps.GetServerAddr().String())
	if err != nil {
		logger.Warn("Failed to resolve ServerAddr", "ServerAddr", ps.GetServerAddr().String(), "Error", err.Error())
		cancel(fmt.Errorf("failed to resolve server addr: %v", err))
		return
	}
	// Listener
	con, err := net.ListenTCP("tcp", pAddr)
	if err != nil {
		logger.Warn("Failed to listen on proxy", "Error", err.Error(), "ProxyAddress", pAddr.String())
		cancel(fmt.Errorf("failed to listen on proxy: %v", err))
		return
	}
	logger.Debug("Listener started")
	for ctx.Err() == nil {
		con.SetDeadline(time.Now().Add(time.Second * 2))
		c, err := con.AcceptTCP()
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
		s, err := net.DialTCP("tcp", nil, sAddr)
		if err != nil {
			logger.Warn("Failed to create new connection to server", "Error", err.Error(), "ServerAddress", sAddr.String())
			c.Close()
			cancel(fmt.Errorf("failed to create new connection to server: %v", err))
			continue
		}
		// Add the proxy in
		logger.Debug("Adding new proxy", "ClientAddr", c.RemoteAddr().String(), "ServerAddr", s.RemoteAddr().String())
		ps.AddConnection(newTcpProxy(c, s))
	}
	// Proxy handler died - no need to cancel.
}
