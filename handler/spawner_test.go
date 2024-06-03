package handler_test

import (
	"context"
	"errors"
	"ezproxy/handler"
	"ezproxy/mocks"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

var (
	errTestDone error = errors.New("test done")
)

func multiTimeAfter(d time.Duration, n uint) <-chan time.Time {
	if n == 0 {
		panic("n must be positive")
	}
	ch := make(chan time.Time)
	go func() {
		v := <-time.After(d)
		for i := uint(0); i != n; i++ {
			ch <- v
		}
	}()
	return ch
}

func TestNewProxySpawnerNoListeners(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(handler.ErrSpawnerClosedOk)
	server := mocks.NewAddr(t)
	proxy := mocks.NewAddr(t)
	_, err := handler.NewProxySpawner(server, proxy, ctx)
	if err == nil {
		t.Fatalf("NewProxySpawner was ok with no listners")
	}
}

func TestNewProxySpawnerCallListeners(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	server := mocks.NewAddr(t)
	proxy := mocks.NewAddr(t)
	pxc := mocks.NewIProxyListner(t)
	waitChan := multiTimeAfter(time.Second*1, 2)
	pxc.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.AnythingOfType("*handler.ProxySpawner")).Run(func(args mock.Arguments) {
		// Close the context
		args[1].(context.CancelCauseFunc)(handler.ErrProxyClosedOk)
	}).Once().WaitUntil(waitChan).Return(waitChan)
	ps, err := handler.NewProxySpawner(server, proxy, ctx, pxc.Execute)
	if err != nil {
		t.Fatalf(err.Error())
	}
	<-waitChan
	ps.Close()
	cancel(errors.New("test done"))
}

func TestNewProxySpawnerDeadContext(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errTestDone)
	server := mocks.NewAddr(t)
	proxy := mocks.NewAddr(t)
	pxc := mocks.NewIProxyListner(t)
	_, err := handler.NewProxySpawner(server, proxy, ctx, pxc.Execute)
	<-time.After(time.Millisecond * 500)
	if err != nil {
		if !errors.Is(err, errTestDone) {
			t.Fatalf("got invalid error, expected %v got %v", errTestDone, err)
		}
	}
}

func TestNewProxySpawnerListenerDoesntCloseContext(t *testing.T) {
	ctx, _ := context.WithCancelCause(context.Background())
	server := mocks.NewAddr(t)
	proxy := mocks.NewAddr(t)
	pxc := mocks.NewIProxyListner(t)
	waitChan := multiTimeAfter(time.Second*1, 3)
	pxc.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.AnythingOfType("*handler.ProxySpawner")).Once().WaitUntil(waitChan).Return()
	ps, err := handler.NewProxySpawner(server, proxy, ctx, pxc.Execute)
	if err != nil {
		t.Fatalf("Error in NewProxySpawner: %v", err)
	}
	<-waitChan
	if ps.GetContext().Err() != nil {
		t.Logf("ctx.Err() %v", context.Cause(ps.GetContext()))
	} else {
		t.Fatalf("Context not cancelled")
	}
}

func TestNewProxySpawnerRetry(t *testing.T) {
	ctx, _ := context.WithCancelCause(context.Background())
	server := mocks.NewAddr(t)
	proxy := mocks.NewAddr(t)
	pxc := mocks.NewIProxyListner(t)
	waitChan := multiTimeAfter(time.Second*1, 3)
	pxc.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.AnythingOfType("*handler.ProxySpawner")).Once().WaitUntil(waitChan).Return()
	ps, err := handler.NewProxySpawner(server, proxy, ctx, pxc.Execute)
	if err != nil {
		t.Fatalf("Error in NewProxySpawner: %v", err)
	}
	<-waitChan
	if ps.GetContext().Err() != nil {
		t.Logf("ctx.Err() %v", context.Cause(ps.GetContext()))
	} else {
		t.Fatalf("Context not cancelled")
	}
}
