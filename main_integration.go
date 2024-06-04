//go:build integration
// +build integration

package main

import (
	"context"
	"errors"
	"ezproxy/integration"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
)

const (
	TestProxyAddr  string = "10.0.0.46:5555"
	TestServerAddr string = "10.0.0.46:5554"
)

var logger *slog.Logger

type threadManager struct {
	wg          sync.WaitGroup
	ctx         context.Context
	cancelCause context.CancelCauseFunc
}

func (t *threadManager) Run(cb func()) {
	awaitStart := make(chan bool)
	go func() {
		t.wg.Add(1)
		awaitStart <- true
		cb()
		t.wg.Done()
	}()
	select {
	case <-awaitStart:
	case <-t.ctx.Done():
		return
	}
}

func (t *threadManager) Join() error {
	wgChan := make(chan bool)
	go func() {
		defer close(wgChan)
		t.wg.Wait()
	}()
	select {
	case <-t.ctx.Done():
		return context.Cause(t.ctx)
	case <-wgChan:
		return nil
	}
}

func NewThreadManager(ctx context.Context) *threadManager {
	subCtx, cancel := context.WithCancelCause(ctx)
	return &threadManager{
		wg:          sync.WaitGroup{},
		ctx:         subCtx,
		cancelCause: cancel,
	}
}

func main() {
	fmt.Printf("Proxy: %s\nAPI: %s\n", versionToString(proxyVersion), versionToString(apiVersion))
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	serverAddr, err := net.ResolveTCPAddr("tcp", TestServerAddr)
	if err != nil {
		logger.Error("Failed to resolve serverAddress, this is a setup error", "Error", err.Error())
		return
	}
	proxyAddr, err := net.ResolveTCPAddr("tcp", TestProxyAddr)
	if err != nil {
		logger.Error("Failed to resolve proxyAddr, this is a setup error", "Error", err.Error())
		return
	}
	ctx, cancelCause := context.WithCancelCause(context.Background())
	defer cancelCause(errors.New("main() closed"))
	tm := NewThreadManager(ctx)
	tm.Run(func() {
		logger.Debug("Starting TcpTestServer")
		err := integration.TcpTestServer(ctx, serverAddr, proxyAddr)
		if err != nil {
			logger.Error("TcpTestServer error", "Error", err.Error(), "Ctx.Error", ctx.Err(), "Cause", context.Cause(ctx))
		}
		logger.Debug("Done TcpTestServer", "Ctx.Error", ctx.Err(), "Cause", context.Cause(ctx))
	})
	if err = tm.Join(); err != nil {
		logger.Error("ThreadManager error", "Error", err.Error())
	}
}
