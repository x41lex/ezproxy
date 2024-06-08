package handler_test

import (
	"bytes"
	"context"
	"errors"
	"ezproxy/handler"
	"ezproxy/mocks"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockAddr struct {
	network  string
	toString string
}

func (m *MockAddr) Network() string {
	return m.network
}

func (m *MockAddr) String() string {
	return m.toString
}

func NewMockAddr(address string) *MockAddr {
	return &MockAddr{
		network:  "test net",
		toString: address,
	}
}

type ListenerCloseType int

const (
	ListenerCloseNormal       ListenerCloseType = iota // Cancel context with ErrProxyClosedOk
	ListenerCloseRetry                                 // Cancel context with ErrProxyRetry
	ListenerCloseError                                 // Cancel context with 'test error'
	ListenerCloseLeaveContext                          // Don't cancel context
)

type MockSpawnerInfo struct {
	Listener        *mocks.IProxyListener    // Listener
	StopListener    chan<- ListenerCloseType // Send true to close the listener & cancel its context, or false to close it without closing the context
	ListenerRunning *bool                    // Is the listener running
	ListenerDone    <-chan struct{}          // Has data when the context closes
	CreateContainer *mocks.CreateIProxyContainer
	Spawner         *handler.ProxySpawner
	Context         context.Context
	Cancel          context.CancelCauseFunc
	ServerAddr      net.Addr
	ProxyAddr       net.Addr
}

func (m *MockSpawnerInfo) Close() {
	m.Cancel(fmt.Errorf("cancel"))
	m.StopListener <- ListenerCloseNormal
}

// Helper function to make a test spawner
func createMockSpawner(t *testing.T) *MockSpawnerInfo {
	ipl := mocks.NewIProxyListener(t)
	// Wait for the callback to be called
	ch := make(chan bool)
	// Close the listener (If it exists early its a error)
	// Send 'true' to close the context
	closeCh := make(chan ListenerCloseType)
	// Notify when the listener is closed
	listenerDoneCh := make(chan struct{})
	isRunning := true
	doSetup := true
	ipl.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.Anything).RunFn = func(a mock.Arguments) {
		isRunning = true
		// This might not be safe on retries so we don't use that.
		cancel := a.Get(1).(context.CancelCauseFunc)
		if doSetup {
			ch <- true
		}
		v := <-closeCh
		switch v {
		case ListenerCloseNormal:
			cancel(handler.ErrProxyClosedOk)
		case ListenerCloseRetry:
			cancel(handler.ErrProxyRetry)
		case ListenerCloseError:
			cancel(errors.New("test error"))
		case ListenerCloseLeaveContext:
			// Do nothing.
		default:
			panic("Unexpected ListenerClose type")
		}
		listenerDoneCh <- struct{}{}
		isRunning = false
	}
	cnt := mocks.NewCreateIProxyContainer(t)
	sAddr := NewMockAddr("Server Net")
	pAddr := NewMockAddr("Proxy Net")
	ctx, cancel := context.WithCancelCause(context.Background())
	ps, err := handler.NewProxySpawnerWithContainer(sAddr, pAddr, cnt.Execute, ctx, ipl.Execute)
	if err != nil {
		t.Fatalf("Failed to create proxy spawner: %v", err)
		return nil
	}
	<-ch
	doSetup = false
	return &MockSpawnerInfo{
		Listener:        ipl,
		StopListener:    closeCh,
		ListenerDone:    listenerDoneCh,
		CreateContainer: cnt,
		Spawner:         ps,
		Context:         ctx,
		Cancel:          cancel,
		ServerAddr:      sAddr,
		ProxyAddr:       pAddr,
		ListenerRunning: &isRunning,
	}
}

// NewProxySpawnerWithContainer, Ensure failure if server addr & proxy addr are the same
//
// Expect: Error in creating spawner
func TestNewSpawnerSameAddr(t *testing.T) {
	sAddr := NewMockAddr("Server Net")
	ctx, cancel := context.WithCancel(context.Background())
	// This shouldn't be executed, a error should be returned way before it can be
	lst := mocks.NewIProxyListener(t)
	defer cancel()
	_, err := handler.NewProxySpawner(sAddr, sAddr, ctx, lst.Execute)
	if err == nil {
		t.Fatalf("No error when creating proxy spawner when server and proxy address are the same")
	}
	if err.Error() != "server address and proxy address must be different" {
		t.Logf("Unexpected error type (String may have changed, this is not a big issue): %v", err)
	}
}

// NewProxySpawnerWithContainer, Ensure failure if there are no listeners
//
// Expect: Error in creating Spawner
func TestNewSpawnerNoListeners(t *testing.T) {
	sAddr := NewMockAddr("Server Net")
	pAddr := NewMockAddr("Proxy Net")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := handler.NewProxySpawner(sAddr, pAddr, ctx)
	if err == nil {
		t.Fatalf("No error when creating proxy spawner with no listeners")
	}
	if err.Error() != "no listeners given" {
		t.Logf("Unexpected error type (String may have changed, this is not a big issue): %v", err)
	}
}

// NewProxySpawnerWithContainer, Ensure failure (after listeners are called) if context is cancelled
//
// Expect: Error in creating spawner
func TestNewProxyCancel(t *testing.T) {
	ipl := mocks.NewIProxyListener(t)
	pAddr := NewMockAddr("Proxy")
	sAddr := NewMockAddr("Server")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := handler.NewProxySpawner(pAddr, sAddr, ctx, ipl.Execute)
	if err == nil {
		t.Fatalf("No error when creating proxy spawner with no listeners")
	}
	if err.Error() != "context canceled" {
		t.Logf("Unexpected error type (String may have changed, this is not a big issue): %v", err)
	}
}

// NewProxySpawnerWithContainer, Create ProxySpawner with listeners (Single)
//
// Expect: Spawner is created, the listener is called and nothing is setup.
func TestNewProxy(t *testing.T) {
	ipl := mocks.NewIProxyListener(t)
	// We can't do much with the arguments - we know there the right type but we cant
	// verify them at all.
	waitCh := make(chan struct{})
	listenCh := make(chan struct{})
	on := ipl.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.Anything)
	on.RunFn = func(a mock.Arguments) {
		waitCh <- struct{}{}
		<-listenCh
	}
	sAddr := NewMockAddr("Server Net")
	pAddr := NewMockAddr("Proxy Net")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ps, err := handler.NewProxySpawner(sAddr, pAddr, ctx, ipl.Execute)
	if err != nil {
		t.Fatalf("Failed to create proxy spawner with listener: %v", err)
	}
	timeout := time.NewTimer(time.Second * 3)
	select {
	case <-timeout.C:
		t.Fatalf("Timed out waiting for listener to be called")
	case <-waitCh:
	}
	if len(ps.GetAllProxies()) != 0 {
		t.Errorf("Expected setup to have no proxies, has %d", len(ps.GetAllProxies()))
	}
	if ps.GetBytesSent() != 0 {
		t.Errorf("Expected setup to have no bytes sent, has %d", ps.GetBytesSent())
	}
	if !ps.IsAlive() {
		ctx := ps.GetContext()
		cause := context.Cause(ctx)
		t.Errorf("Expected setup to be alive. Ctx.Err: %v Cause(ctx): %v", ctx.Err(), cause)
	}
}

// NewProxySpawnerWithContainer, Create ProxySpawner with listeners (Multiple)
//
// Expect: Spawner is created & all listeners is called
func TestNewProxyMultiple(t *testing.T) {
	wg := sync.WaitGroup{}
	ipl1 := mocks.NewIProxyListener(t)
	ipl1.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.Anything).RunFn = func(a mock.Arguments) {
		wg.Done()
		ctx := a.Get(0).(context.Context)
		<-ctx.Done()
	}
	ipl2 := mocks.NewIProxyListener(t)
	ipl2.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.Anything).RunFn = func(a mock.Arguments) {
		wg.Done()
		ctx := a.Get(0).(context.Context)
		<-ctx.Done()
	}
	ipl3 := mocks.NewIProxyListener(t)
	ipl3.On("Execute", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("context.CancelCauseFunc"), mock.Anything).RunFn = func(a mock.Arguments) {
		wg.Done()
		ctx := a.Get(0).(context.Context)
		<-ctx.Done()
	}
	sAddr := NewMockAddr("Server Net")
	pAddr := NewMockAddr("Proxy Net")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg.Add(3)
	_, err := handler.NewProxySpawner(sAddr, pAddr, ctx, ipl1.Execute, ipl2.Execute, ipl3.Execute)
	if err != nil {
		t.Fatalf("Failed to create proxy spawner with listener: %v", err)
	}
	wg.Wait()
}

// NewProxySpawnerWithContainer, Ensure `IProxyContainer` is called
//
// Expect: Container is called & A proxy container is returned
func TestProxyContainerCalled(t *testing.T) {
	ms := createMockSpawner(t)
	defer ms.Close()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(mocks.NewIProxyContainer(t), nil)
	//defer ms.Cancel(fmt.Errorf("context cancelled"))
	px := mocks.NewIProxy(t)
	_, err := ms.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}

}

// AddConnection, `p.context` is dead (Fails)
//
// Expect: returns error
func TestAddConnectionDeadCtx(t *testing.T) {
	ms := createMockSpawner(t)
	ps := ms.Spawner
	defer ms.Close()
	ms.Cancel(fmt.Errorf("test cancel"))
	px := mocks.NewIProxy(t)
	if len(ps.GetAllProxies()) != 0 {
		t.Fatalf("Expected 0 proxies got %d", len(ps.GetAllProxies()))
	}
	_, err := ps.AddConnection(px)
	if err == nil {
		t.Fatalf("Expected error when adding connection to dead context")
	}
	if len(ps.GetAllProxies()) != 0 {
		t.Fatalf("Expected 0 proxies got %d", len(ps.GetAllProxies()))
	}
}

// AddConnection, `p.containerMaker` returns a error (Fails)
//
// Expect: AddConnection fails, but the spawner isn't killed
func TestAddConnectionContainerMakerError(t *testing.T) {
	ms := createMockSpawner(t)
	defer ms.Close()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(nil, errors.New("test container error"))
	px := mocks.NewIProxy(t)
	_, err := ms.Spawner.AddConnection(px)
	if err == nil {
		t.Fatalf("AddConnection didn't fail when ContainerMaker returned a error")
	}
	t.Logf("AddConnection failed with \"%v\"", err)
	if !ms.Spawner.IsAlive() {
		t.Fatalf("Spawner died when ContainerMaker returned a error")
	}
}

// AddConnection, Ensure Proxy Id increments and starts at 0 && Ensure proxy exists
//
// Expect: Proxy ID starts at 0, increments per proxy, and each proxy actually exists
func TestAddConnection(t *testing.T) {
	ms := createMockSpawner(t)
	defer ms.Close()
	pc1 := mocks.NewIProxyContainer(t)
	pc1.On("GetId").Return(0)
	// Just in case the pruner runs
	pc1.On("IsAlive").Return(true).Maybe()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc1, nil)
	pc2 := mocks.NewIProxyContainer(t)
	pc2.On("GetId").Return(1)
	pc2.On("IsAlive").Return(true).Maybe()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 1).Return(pc2, nil)
	pc3 := mocks.NewIProxyContainer(t)
	pc3.On("GetId").Return(2)
	pc3.On("IsAlive").Return(true).Maybe()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 2).Return(pc3, nil)
	px1 := mocks.NewIProxy(t)
	_, err := ms.Spawner.AddConnection(px1)
	if err != nil {
		t.Fatalf("Failed to add connection 1: %v", err)
	}
	px2 := mocks.NewIProxy(t)
	_, err = ms.Spawner.AddConnection(px2)
	if err != nil {
		t.Fatalf("Failed to add connection 2: %v", err)
	}
	px3 := mocks.NewIProxy(t)
	_, err = ms.Spawner.AddConnection(px3)
	if err != nil {
		t.Fatalf("Failed to add connection 3: %v", err)
	}
	pxs := ms.Spawner.GetAllProxies()
	for k := range pxs {
		px, err := ms.Spawner.GetProxy(k)
		if err != nil {
			t.Errorf("Failed to get proxy '%d': %v", k, err)
			continue
		}
		gId := px.GetId()
		if gId != k {
			t.Errorf("Got wrong proxy, expected %d got %d", k, gId)
		}
	}
}

// AddConnection, Ensure pruning works
//
// Expect: Ids are never reused even after pruning.
func TestAddConnectionPrune(t *testing.T) {
	waitForPruner := make(chan bool)
	ms := createMockSpawner(t)
	defer ms.Close()
	pc1 := mocks.NewIProxyContainer(t)
	pc1.On("GetId").Return(0)
	pc1.On("IsAlive").Return(true)
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc1, nil)
	pc2 := mocks.NewIProxyContainer(t)
	pc2.On("GetId").Return(1).Maybe()
	pc2.On("IsAlive").Return(false).RunFn = func(a mock.Arguments) {
		waitForPruner <- true
	}
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 1).Return(pc2, nil)
	pc3 := mocks.NewIProxyContainer(t)
	pc3.On("GetId").Return(2)
	pc3.On("IsAlive").Return(true)
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 2).Return(pc3, nil)
	pc4 := mocks.NewIProxyContainer(t)
	pc4.On("GetId").Return(3)
	// The pruner might not run on these.
	pc4.On("IsAlive").Return(true).Maybe()
	ms.CreateContainer.On("Execute", mock.Anything, mock.Anything, 3).Return(pc4, nil)
	px1 := mocks.NewIProxy(t)
	_, err := ms.Spawner.AddConnection(px1)
	if err != nil {
		t.Fatalf("Failed to add connection 1: %v", err)
	}
	px2 := mocks.NewIProxy(t)
	_, err = ms.Spawner.AddConnection(px2)
	if err != nil {
		t.Fatalf("Failed to add connection 2: %v", err)
	}
	px3 := mocks.NewIProxy(t)
	_, err = ms.Spawner.AddConnection(px3)
	if err != nil {
		t.Fatalf("Failed to add connection 3: %v", err)
	}
	<-waitForPruner
	px4 := mocks.NewIProxy(t)
	_, err = ms.Spawner.AddConnection(px4)
	if err != nil {
		t.Fatalf("Failed to add connection 4: %v", err)
	}
	for k := range 4 {
		px, err := ms.Spawner.GetProxy(k)
		if k == 1 {
			if err == nil {
				t.Errorf("Dead proxy should be deleted")
			}
			continue
		}
		if err != nil {
			t.Errorf("Failed to get proxy '%d': %v", k, err)
			continue
		}
		gId := px.GetId()
		if gId != k {
			t.Errorf("Got wrong proxy, expected %d got %d", k, gId)
		}
	}
}

func TestHandleTestAddr(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(NewMockAddr("Client"))
	pc.On("GetServerAddr").Return(NewMockAddr("Server"))
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err := si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	r := si.Spawner.HandleSend([]byte("HELLO WORLD"), 0, pc)
	if !r {
		t.Fatalf("Invalid HandleSend result")
	}
}

// HandleSend, Ensure that if `sendCallback` is `nil` the return value is true
//
// Expect: Return of HandleSend is true
func TestHandleNoCallback(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err := si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	r := si.Spawner.HandleSend([]byte("HELLO WORLD"), 0, pc)
	if !r {
		t.Fatalf("Invalid HandleSend result")
	}
}

// HandleSend, Ensure `sendCallback` is called on send && Ensure `sendCallbacks` return is respected
//
// Expect: SendCallback is called and return is true
func TestHandleCallbackCalled(t *testing.T) {
	pCb := mocks.NewPacketSendCallback(t)
	pCb.On("Execute", mock.Anything, mock.Anything, mock.Anything).Return(true)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(NewMockAddr("Client"))
	pc.On("GetServerAddr").Return(NewMockAddr("Server"))
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, si.Context)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	r := si.Spawner.HandleSend([]byte("HELLO WORLD"), 0, pc)
	if !r {
		t.Fatalf("Invalid HandleSend result")
	}
	pCb.On("Execute", mock.Anything, mock.Anything, mock.Anything).Return(false)
}

// HandleSend, Ensure `sendCallback` is called on send && Ensure `sendCallbacks` return is respected
//
// Expect: SendCallback is called and return is false
func TestHandleCallbackDrop(t *testing.T) {
	pCb := mocks.NewPacketSendCallback(t)
	pCb.On("Execute", mock.Anything, mock.Anything, mock.Anything).Return(false)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, si.Context)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	r := si.Spawner.HandleSend([]byte("HELLO WORLD"), 0, pc)
	if r {
		t.Fatalf("Invalid HandleSend result")
	}
}

// HandleSend, Ensure packets with the `handler.CapFlag_Injected` bit set cannot be dropped
//
// Expect: HandleSend returns true
func TestHandleCallbackDropInjected(t *testing.T) {
	pCb := mocks.NewPacketSendCallback(t)
	pCb.On("Execute", mock.Anything, handler.CapFlag_Injected, mock.Anything).Return(false)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, si.Context)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	r := si.Spawner.HandleSend([]byte("HELLO WORLD"), handler.CapFlag_Injected, pc)
	if !r {
		t.Fatalf("Invalid HandleSend result")
	}
}

// TrySetFilterCallback, Ensure you can set a filter after a previous filter if its context was cancelled && Ensure you can set filter callbacks
//
// Expect: A second callback should be allowed after the first is cancelled.
func TestTrySetFilterCallbackCallbackReset(t *testing.T) {
	pCb := mocks.NewPacketSendCallback(t)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("IsAlive").Return(true).Maybe()
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	ctx1, cancel1 := context.WithCancel(si.Context)
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, ctx1)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	cancel1()
	ctx2, c2 := context.WithCancel(si.Context)
	defer c2()
	err = si.Spawner.TrySetFilterCallback(pCb.Execute, ctx2)
	if err != nil {
		t.Fatalf("Failed to reset filter callback: %v", err)
	}
}

// TrySetFilterCallback, Don't allow setting a callback if one already exists && Ensure you can set filter callbacks
//
// Expect: A second callback should not be allowed after the first is cancelled.
func TestHandleCallbackCantReset(t *testing.T) {
	pCb := mocks.NewPacketSendCallback(t)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("IsAlive").Return(true).Maybe()
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	ctx1, c1 := context.WithCancel(si.Context)
	defer c1()
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, ctx1)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	//cancel1()
	ctx2, c2 := context.WithCancel(si.Context)
	defer c2()
	err = si.Spawner.TrySetFilterCallback(pCb.Execute, ctx2)
	if err == nil {
		t.Fatalf("Shouldn't be able to set callback if it is already set")
	}
}

// TrySetFilterCallback, Ensure when `callbackCtx` is cancelled the callback is correctly removed
//
// Expect: When callbackCtx is cancelled the callback wont be called.
func TestHandleCallbackCancelledContext(t *testing.T) {
	// pCb.Execute shouldn't be called
	pCb := mocks.NewPacketSendCallback(t)
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetId").Return(0).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// We aren't testing TrySetFilterCallback here
	ctx, cancel := context.WithCancel(si.Context)
	err := si.Spawner.TrySetFilterCallback(pCb.Execute, ctx)
	if err != nil {
		t.Fatalf("Failed to set filter callback: %v", err)
	}
	cancel()
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err = si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	si.Spawner.HandleSend([]byte{0, 1, 2, 3, 4}, 0, pc)
}

// GetContext, Ensure it returns the correct context
//
// Expect: Context is cancelled with the correct cause
func TestGetContext(t *testing.T) {
	ms := createMockSpawner(t)
	defer ms.Close()
	ctx := ms.Spawner.GetContext()
	if ctx.Err() != nil {
		t.Fatalf("Context was closed")
	}
	cancelErr := errors.New("TestGetContext cancel")
	ms.Cancel(cancelErr)
	if ctx.Err() == nil {
		t.Fatalf("Context wasn't closed")
	}
	cause := context.Cause(ctx)
	if cause.Error() != cancelErr.Error() {
		t.Fatalf("Invalid cancel cause, got '%s' expected '%s'", ctx.Err().Error(), cancelErr.Error())
	}
}

// GetProxyAddr && GetServerAddr, Ensure return is correct
//
// Expect: Ensure return is correct
func TestGetAddrs(t *testing.T) {
	ms := createMockSpawner(t)
	defer ms.Close()
	pAddr := ms.Spawner.GetProxyAddr()
	sAddr := ms.Spawner.GetServerAddr()
	if pAddr.Network() != ms.ProxyAddr.Network() || pAddr.String() != ms.ProxyAddr.String() {
		t.Errorf("Incorrect proxy address, got %+v expected %+v", pAddr, ms.ProxyAddr)
	}
	if sAddr.Network() != ms.ServerAddr.Network() || sAddr.String() != ms.ServerAddr.String() {
		t.Errorf("Incorrect proxy address, got %+v expected %+v", sAddr, ms.ServerAddr)
	}
}

// GetBytesSent, Ensure return if correct
//
// Expect: Ensure return is correct
func TestGetBytesSent(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(pc, nil)
	// Now we need to create a container
	px := mocks.NewIProxy(t)
	_, err := si.Spawner.AddConnection(px)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}
	data := []byte("HELLO WORLD")
	r := si.Spawner.HandleSend(data, 0, pc)
	if !r {
		t.Fatalf("Invalid HandleSend result")
	}
	bg := si.Spawner.GetBytesSent()
	if bg != uint64(len(data)) {
		t.Fatalf("Got invalid number of bytes, expected %d got %d", len(data), bg)
	}
}

// GetRecvChan, Ensure a unique channel is got
//
// Expect: Ensure channels are unique
func TestGetRecvChanUnique(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	c1, rCtx1, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	_ = rCtx1
	defer rCtxCan1()
	c2, rCtx2, rCtxCan2 := si.Spawner.GetRecvChan(si.Context)
	_ = rCtx2
	defer rCtxCan2()
	if c1 == c2 {
		t.Errorf("Returned channel was the same")
	}
}

// GetRecvChan, Ensure channels stop getting data after the context dies
//
// Expect: Channel should get no data after context is closed
func TestGetRecvChanCloses(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	c1, _, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(0).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	pc.On("GetClientAddr").Return(&MockAddr{})
	pc.On("GetServerAddr").Return(&MockAddr{})
	defer rCtxCan1()
	rCtxCan1()
	failChan := make(chan bool)
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		select {
		case <-c1:
			failChan <- true
		case <-ticker.C:
			failChan <- false
		}
	}()
	si.Spawner.HandleSend([]byte("HELLO WORLD"), 0, pc)
	v := <-failChan
	if v {
		t.Fatalf("Got data on channel that should be closed")
	}
}

func pktChanDataOk(t *testing.T, pkt *handler.PacketChanData, data []byte, flags uint32, source net.Addr, dest net.Addr, proxyId int) {
	if !bytes.Equal(pkt.Data, data) {
		t.Errorf("Got invalid data, expected '%s' (%x), got '%s' (%x)", data, data, pkt.Data, pkt.Data)
	}
	if pkt.Flags != handler.CapFlags(flags) {
		t.Errorf("Got invalid flags, expected %d, got %d", flags, pkt.Flags)
	}
	if pkt.ProxyId != proxyId {
		t.Errorf("Got invalid packet id, expected %d, got %d", proxyId, pkt.ProxyId)
	}
	if pkt.Dest.String() != source.String() || pkt.Dest.Network() != source.Network() {
		t.Errorf("Got invalid destination, expected %+v, got %+v", source, pkt.Dest)
	}
	if pkt.Source.String() != dest.String() || pkt.Source.Network() != dest.Network() {
		t.Errorf("Got invalid source, expected %+v, got %+v", dest, pkt.Source)
	}
}

// GetRecvChan, Ensure channels stop getting data after the context dies
//
// Expect: Channel should get no data after context is closed
func TestGetRecvChanGetsData(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	c1, _, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(5).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	cl_addr := NewMockAddr("TestClient")
	pc.On("GetClientAddr").Return(cl_addr)
	sv_addr := NewMockAddr("TestServer")
	pc.On("GetServerAddr").Return(sv_addr)
	defer rCtxCan1()
	failChan := make(chan *handler.PacketChanData)
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		select {
		case d := <-c1:
			failChan <- &d
		case <-ticker.C:
			failChan <- nil
		}
	}()
	test_data := []byte("HELLO WORLD")
	si.Spawner.HandleSend(test_data, handler.CapFlag_ToServer, pc)
	v := <-failChan
	if v == nil {
		t.Fatalf("Got no data on channel")
	}
	pktChanDataOk(t, v, test_data, uint32(handler.CapFlag_ToServer), sv_addr, cl_addr, 5)
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		select {
		case d := <-c1:
			failChan <- &d
		case <-ticker.C:
			failChan <- nil
		}
	}()
	test_data = []byte("FSDKMOFDSIUHSDFIUGDFSNIUGFDJN IRUFDGNGF DJNFGDIU H*(RTWEJ$E*(UH$*( JRGTIUEh485 9hj 43fa08-943 )))")
	si.Spawner.HandleSend(test_data, handler.CapFlag_Injected, pc)
	v = <-failChan
	if v == nil {
		t.Fatalf("Got no data on channel")
	}
	pktChanDataOk(t, v, test_data, uint32(handler.CapFlag_Injected), cl_addr, sv_addr, 5)
}

// GetRecvChan, Ensure `IsAlive` is still true after the context is closed
//
// Expect: Channel context should not close
func TestGetRecvChanDontCloseContext(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	_, _, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	rCtxCan1()
	if !si.Spawner.IsAlive() {
		t.Fatalf("Spawner context closed after callback was closed")
	}
}

// GetRecvChan, Ensure channels stop getting data after the context dies
//
// Expect: Channel should get no data after context is closed
func TestGetRecvChanIgnorePackets(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	c1, _, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(5).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	cl_addr := NewMockAddr("TestClient")
	pc.On("GetClientAddr").Return(cl_addr)
	sv_addr := NewMockAddr("TestServer")
	pc.On("GetServerAddr").Return(sv_addr)
	defer rCtxCan1()
	failChan := make(chan *handler.PacketChanData)
	test_data := []byte("HELLO WORLD")
	si.Spawner.HandleSend(test_data, handler.CapFlag_ToServer, pc)
	test_data = []byte("FSDKMOFDSIUHSDFIUGDFSNIUGFDJN IRUFDGNGF DJNFGDIU H*(RTWEJ$E*(UH$*( JRGTIUEh485 9hj 43fa08-943 )))")
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		select {
		case d := <-c1:
			failChan <- &d
		case <-ticker.C:
			failChan <- nil
		}
	}()
	si.Spawner.HandleSend(test_data, handler.CapFlag_Injected, pc)
	v := <-failChan
	if v == nil {
		t.Fatalf("Got no data on channel")
	}
	pktChanDataOk(t, v, test_data, uint32(handler.CapFlag_Injected), cl_addr, sv_addr, 5)
}

// GetRecvChan, Ensure data is sent correctly with a filterer
//
// This tests can be finicky at times.
//
// Expect: Correct data is sent when a filterer is set if true is returned, and not if false is returned
func TestGetRecvChanFilter(t *testing.T) {
	testCb := func(data []byte, flags handler.CapFlags, pktCnt handler.IProxyContainer) bool {
		// Only allow packets with 0x0 as there first byte
		return data[0] == 0
	}
	si := createMockSpawner(t)
	defer si.Close()
	si.Spawner.TrySetFilterCallback(testCb, si.Context)
	c1, _, rCtxCan1 := si.Spawner.GetRecvChan(si.Context)
	pc := mocks.NewIProxyContainer(t)
	pc.On("GetId").Return(5).Maybe()
	pc.On("IsAlive").Return(true).Maybe()
	cl_addr := NewMockAddr("TestClient")
	pc.On("GetClientAddr").Return(cl_addr)
	sv_addr := NewMockAddr("TestServer")
	pc.On("GetServerAddr").Return(sv_addr)
	defer rCtxCan1()
	failChan := make(chan *handler.PacketChanData, 5)
	go func() {
		ticker := time.NewTicker(time.Millisecond * 50)
		select {
		case d := <-c1:
			failChan <- &d
		case <-ticker.C:
			failChan <- nil
		}
	}()
	test_data := []byte("\x00HELLO WORLD")
	// Should be allowed
	si.Spawner.HandleSend(test_data, handler.CapFlag_ToServer, pc)
	v := <-failChan
	if v == nil {
		t.Errorf("Didn't get data on recvChan for allowed packet")
	}
	go func() {
		ticker := time.NewTicker(time.Second * 1)
		for {
			select {
			case d := <-c1:
				failChan <- &d
			case <-ticker.C:
				failChan <- nil
			}
		}
	}()
	// Shouldn't be dropped (Injected)
	test_data = []byte("FSDKMOFDSIUHSDFIUGDFSNIUGFDJN IRUFDGNGF DJNFGDIU H*(RTWEJ$E*(UH$*( JRGTIUEh485 9hj 43fa08-943 )))")
	si.Spawner.HandleSend(test_data, handler.CapFlag_Injected, pc)
	v = <-failChan
	if v == nil {
		t.Errorf("Didn't get data on recvChan for injected disallowed packet")
	}
	// Should be dropped
	test_data = []byte("fdi9grs895rsj98 resn894g5es80nr eg9unv9eur n4e9run-sdfsh80jdsf0-da")
	si.Spawner.HandleSend(test_data, handler.CapFlag_ToServer, pc)
	v = <-failChan
	if v != nil {
		t.Errorf("Got data on recvChan for allowed packet")
	}
	// Should be allowed
	test_data = []byte("\x00fdi9grs895rsj98 resn894g5es80nr eg9unv9eur n4e9run-sdfsh80jdsf0-da")
	si.Spawner.HandleSend(test_data, 0, pc)
	v = <-failChan
	if v == nil {
		t.Fatalf("Didn't get data on channel")
	}
}

// CloseProxy, Ensure you can only close valid proxies
//
// Expect: Errors on close
func TestCloseProxyInvalid(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	for v := range 50 {
		err := si.Spawner.CloseProxy(v)
		if err == nil {
			t.Errorf("Was able to close: %d", v)
		}
	}
}

// CloseProxy, Ensure proxies contexts are correctly closed with `handler.ErrProxyClosedOk` && Ensure only expected proxies are closed
//
// Expect: Errors on close
func TestCloseProxy(t *testing.T) {
	si := createMockSpawner(t)
	defer si.Close()
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 0).Return(mocks.NewIProxyContainer(t), nil)
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 1).Return(mocks.NewIProxyContainer(t), nil)
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 2).Return(mocks.NewIProxyContainer(t), nil)
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 3).Return(mocks.NewIProxyContainer(t), nil)
	si.CreateContainer.On("Execute", mock.Anything, mock.Anything, 4).Return(mocks.NewIProxyContainer(t), nil)
	// Helper for adding proxies
	makeProxy := func(expectClose bool) {
		px := mocks.NewIProxy(t)
		pc, err := si.Spawner.AddConnection(px)
		if err != nil {
			panic(fmt.Sprintf("Failed to AddConnection: %v", err))
		}
		if expectClose {
			tpc := pc.(*mocks.IProxyContainer)
			tpc.On("Cancel", mock.Anything).Return(nil)
		}
	}
	for k := range 5 {
		makeProxy(k == 0 || k == 3)
	}
	// This should work
	si.Spawner.CloseProxy(0)
	si.Spawner.CloseProxy(3)
}

// Close, Ensure the context is cancelled with `handler.ErrSpawnerClosedOk`
//
// Expect: Context closes
func TestClose(t *testing.T) {
	si := createMockSpawner(t)
	ctx := si.Spawner.GetContext()
	if ctx.Err() != nil {
		t.Fatalf("Context closed before .Close: %v", ctx.Err())
	}
	si.Spawner.Close()
	if ctx.Err() == nil {
		t.Fatalf("Context not closed after .Close")
	}
	cause := context.Cause(ctx)
	if !errors.Is(cause, handler.ErrSpawnerClosedOk) {
		t.Errorf("Invalid close, should have been '%v' was '%v' (%v)", handler.ErrSpawnerClosedOk, cause, ctx.Err())
	}
}

// Close, Ensure all recv channels contexts are closed
//
// Expect: Contexts closes
func TestCloseRecvCtx(t *testing.T) {
	si := createMockSpawner(t)
	_, rCtx1, _ := si.Spawner.GetRecvChan(si.Context)
	_, rCtx2, _ := si.Spawner.GetRecvChan(si.Context)
	if rCtx1.Err() != nil || rCtx2.Err() != nil {
		t.Fatalf("Context closed before .Close: 1: %v 2: %v", rCtx1.Err(), rCtx2.Err())
	}
	si.Spawner.Close()
	if rCtx1.Err() == nil || rCtx2.Err() == nil {
		t.Fatalf("Context closed before .Close: 1: %v 2: %v", rCtx1.Err(), rCtx2.Err())
	}
}

// Misc, Ensure the context is cancelled and a error is returned if the listener is closed without closing its context
//
// Expect: Context cancelled
func TestListenerEarlyReturnNoContext(t *testing.T) {
	ms := createMockSpawner(t)
	// Close the listener without closing the context
	ms.StopListener <- ListenerCloseLeaveContext
	// We do need to wait for the listener to be closed though.
	<-ms.ListenerDone
	if ms.Spawner.IsAlive() {
		t.Fatalf("Listener closing without return didn't fail")
	}
}

// Misc, Ensure the spawner context isn't cancelled if a listener cancels with `ErrProxyClosedOk`
//
// Expect: Spawner doesn't close if the proxy closes with ErrProxyClosedOk
func TestCancelListenerOk(t *testing.T) {
	ms := createMockSpawner(t)
	// Close the listener with ErrProxyClosedOk
	ms.StopListener <- ListenerCloseNormal
	<-ms.ListenerDone
	// TODO: This will stop working when 'Ensure the spawner context is closed if all listeners are closed and all proxies are closed.'
	// is implemented
	select {
	case <-ms.Spawner.GetContext().Done():
		t.Fatalf("Context was closed after listener was closed with ErrProxyClosedOk")
	case <-time.After(time.Millisecond * 500):
		// Worst case level wait
	}
}

// Misc, Ensure the spawner context is cancelled if a listener cancels with any other error
//
// Expect: Spawner closes if listener closes with anything other then ErrProxyClosedOk or ErrProxyRetry
func TestCancelListenerError(t *testing.T) {
	ms := createMockSpawner(t)
	// Close the listener with ErrProxyClosedOk
	ms.StopListener <- ListenerCloseError
	<-ms.ListenerDone
	select {
	case <-ms.Spawner.GetContext().Done():
	case <-time.After(time.Millisecond * 500):
		// Worst case level wait
		t.Fatalf("Context wasn't closed after listener was closed with a error")
	}
}

// Misc, Ensure listeners are retired with `ErrProxyRetry` & cancels after max retries (3)
//
// Expect: Retry listener 3 times, then fail
func TestCancelListenerRetry(t *testing.T) {
	ms := createMockSpawner(t)
	// We should get 3 retries
	for range 3 {
		ms.StopListener <- ListenerCloseRetry
		<-ms.ListenerDone
		select {
		case <-ms.Spawner.GetContext().Done():
			t.Fatalf("Context was closed after listener was closed with ListenerCloseRetry")
		case <-time.After(time.Millisecond * 500):
		}
		if !*ms.ListenerRunning {
			t.Fatalf("Listener was not retried")
		}
	}
	// This should fail even if we give it retry.
	ms.StopListener <- ListenerCloseRetry
	<-ms.ListenerDone
	select {
	case <-ms.Spawner.GetContext().Done():
	case <-time.After(time.Second):
		t.Fatalf("Context wasn't closed after listener was closed with ListenerCloseRetry 3 times")
	}
	if *ms.ListenerRunning {
		t.Fatalf("Listener is retried")
	}
}
