//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"ezproxy/handler"
	"ezproxy/proxy"
	"fmt"
	"log/slog"
	"net"
	"time"
)

const (
	echoData string = "HELLO TCP"
)

func tcpEchoConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	logger := slog.Default()
	for ctx.Err() == nil {
		logger.Info("Connection started", "Client", c.RemoteAddr())
		c.SetReadDeadline(time.Now().Add(time.Second))
		buffer := make([]byte, 1024)
		n, err := c.Read(buffer)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			logger.Warn("Failed to read from client, connection closing", "From", c.RemoteAddr().String(), "Error", err.Error())
			return
		}
		logger.Debug("Echoing data", "Data", buffer[:n], "From", c.RemoteAddr().String())
		wrote, err := c.Write(buffer[:n])
		if err != nil {
			logger.Warn("Failed to write data to client, connection closing", "To", c.RemoteAddr().String(), "Error", err.Error())
			return
		}
		if wrote != n {
			logger.Warn("Didn't write correct number of bytes", "Read", n, "Wrote", wrote)
		}
	}
	logger.Info("Connection closing", "Error", ctx.Err().Error())
}

func tcpEchoServer(ctx context.Context, cancel context.CancelCauseFunc, serverAddr net.Addr, setupChan chan<- bool) {
	logger := slog.Default()
	tcpServerAddr, err := addrToTcpAddr(serverAddr)
	if err != nil {
		logger.Error("Failed to resolve serverAddress just passed. This is a setup error", "Error", err.Error(), "ServerAddress", serverAddr)
		cancel(err)
		return
	}
	listen, err := net.ListenTCP("tcp", tcpServerAddr)
	if err != nil {
		logger.Error("Failed to listen", "Error", err.Error())
		cancel(err)
		return
	}
	logger.Info("Echo server listening")
	defer listen.Close()
	setupChan <- true
	for ctx.Err() == nil {
		listen.SetDeadline(time.Now().Add(time.Second))
		c, err := listen.AcceptTCP()
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			logger.Warn("AcceptTCP() Failed", "Error", err)
			continue
		}
		logger.Debug("Accepted new connection", "From", c.RemoteAddr().String())
		go tcpEchoConn(ctx, c)
	}
}

func TcpTestServer(ctx context.Context, serverAddr net.Addr, proxyAddr net.Addr) error {
	logger := slog.Default()
	pxAddr, err := addrToTcpAddr(proxyAddr)
	if err != nil {
		return err
	}
	listenerSetup := make(chan bool)
	ctx, cancel := context.WithCancelCause(ctx)
	go tcpEchoServer(ctx, cancel, serverAddr, listenerSetup)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-listenerSetup:
		break
	case <-time.After(time.Second * 2):
		cancel(fmt.Errorf("Listener timed out"))
		return fmt.Errorf("Listener timed out")
	}
	logger.Info("Creating new spawner")
	spawner, err := handler.NewProxySpawner(serverAddr, proxyAddr, ctx, proxy.TcpListener)
	if err != nil {
		cancel(err)
		logger.Error("Failed to setup ProxySpawner", "Error", err.Error(), "ServerAddress", serverAddr)
		return err
	}
	spawner.SetErrorCallback(func(err error, pc handler.IProxyContainer) {
		logger.Error("TCPSpawner error", "Error", err.Error(), "Container", pc)
	})
	err = spawner.TrySetFilterCallback(func(data []byte, flags handler.CapFlags, proxy handler.IProxyContainer) (shouldSend bool) {
		fmt.Printf("Got %d bytes\n", len(data))
		return true
	}, ctx)
	if err != nil {
		panic(err)
	}
	logger.Info("Connecting to proxy")
	// Start a connection
	con, err := net.DialTCP("tcp", nil, pxAddr)
	if err != nil {
		cancel(err)
		return err
	}
	defer con.Close()
	logger.Info("Sending data")
	n, err := con.Write([]byte(echoData))
	if err != nil {
		cancel(err)
		return err
	}
	logger.Info("Sent echo data")
	if n != len(echoData) {
		cancel(errors.New("didn't send enough data"))
		return errors.New("didn't send enough data")
	}
	logger.Info("Getting data")
	buffer := make([]byte, 1024)
	n, err = con.Read(buffer)
	if err != nil {
		cancel(err)
		return err
	}
	logger.Info("Got data")
	if !bytes.Equal(buffer[:n], []byte(echoData)) {
		cancel(errors.New("echo data not equal"))
		return errors.New("echo data not equal")
	}
	logger.Info("Data equal")
	cancel(errors.New("testing completed"))
	err = spawner.Close()
	if err != nil {
		logger.Error("Failed to close spawner", "Error", err.Error())
		return err
	}
	logger.Info("Testing completed and closed OK")
	return nil
}
