package handler_test

import (
	"context"
	"errors"
	"ezproxy/handler"
	"ezproxy/mocks"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

type ProxyContainerInfo struct {
	Container    *handler.ProxyContainer
	Proxy        *mocks.IProxy
	Spawner      *mocks.IProxySpawner
	Ctx          context.Context
	Cancel       context.CancelFunc
	ProxyContext context.Context
	ProxyCancel  context.CancelCauseFunc
	PktChan      chan<- handler.ProxyPacketData
}

func NewProxyContainer(t *testing.T, clientAddr *MockAddr, id int) *ProxyContainerInfo {
	tCtx, tCan := context.WithCancel(context.Background())
	sp := mocks.NewIProxySpawner(t)
	sp.On("GetContext").Return(tCtx)
	px := mocks.NewIProxy(t)
	pci := &ProxyContainerInfo{
		Proxy:   px,
		Spawner: sp,
		Ctx:     tCtx,
		Cancel:  tCan,
	}
	// This is called for logging - you can ignore it or remove it later.
	px.On("GetClientAddr").Return(clientAddr).Maybe()
	// pktChan chan<- ProxyPacketData, ctx context.Context, cancel context.CancelCauseFunc -> error
	px.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil).RunFn = func(a mock.Arguments) {
		pci.PktChan = a.Get(0).(chan<- handler.ProxyPacketData)
		pci.ProxyContext = a.Get(1).(context.Context)
		pci.ProxyCancel = a.Get(2).(context.CancelCauseFunc)
	}
	pc, err := handler.NewProxyContainer(sp, px, id)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	pci.Container = pc.(*handler.ProxyContainer)
	return pci
}

// NewProxyContainer, Creates ok, arguments are passed ok, context is created from parent context
//
// Expect: Proxy container made ok
func TestNewProxyContainerArguments(t *testing.T) {
	NewProxyContainer(t, NewMockAddr("ClientAddr"), 0)
}

// NewProxyContainer, Fails if `IProxy.Init` returns a error
//
// Expect: Proxy container fails to be created.
func TestNewProxyContainerArgumentsFail(t *testing.T) {
	tCtx, c := context.WithCancel(context.Background())
	defer c()
	sp := mocks.NewIProxySpawner(t)
	sp.On("GetContext").Return(tCtx)
	px := mocks.NewIProxy(t)
	// This is called for logging - you can ignore it or remove it later.
	px.On("GetClientAddr").Return(NewMockAddr("TestClient")).Maybe()
	px.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("Test error"))
	_, err := handler.NewProxyContainer(sp, px, 0)
	if err == nil {
		t.Fatalf("Created error even though it should have failed")
	}
}

// GetId, Ensure correct ID is returned
//
// Expect: Correct ID
func TestGetId(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("ClientAddr"), 5)
	if pci.Container.GetId() != 5 {
		t.Fatalf("Invalid id, expected 5 got %d", pci.Container.GetId())
	}
}

// Network, Ensure correct network is returned from proxy
//
// Expect: Correct Network
func TestGetNetwork(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("ClientAddr"), 1)
	pci.Proxy.On("Network").Return("TestNet")
	net := pci.Container.Network()
	if net != "TestNet" {
		t.Fatalf("Expected 'TestNet' got '%s'", net)
	}
}

func TestIsAlive(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	if !pci.Container.IsAlive() {
		t.Errorf("Container was closed when created")
	}
	pci.Cancel()
	if pci.Container.IsAlive() {
		t.Errorf("Container wasn't closed when context was cancelled")
	}
}

func TestCancel(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	if !pci.Container.IsAlive() {
		t.Errorf("Container was closed when created")
	}
	pci.Container.Cancel(fmt.Errorf("test"))
	if pci.Container.IsAlive() {
		t.Errorf("Container wasn't closed when context was cancelled")
	}
	if pci.ProxyContext.Err() == nil {
		t.Errorf("Proxy context wasn't closed with container context")
	}
}

func TestProxyGetBytesSent(t *testing.T) {
	toServer1 := []byte("TO_SERVER_1")
	toClient1 := []byte("TO_CLIENT_1")
	toServer2 := []byte("TO_SERVER_2")
	toClient2 := []byte("TO_SERVER_2")
	expectedValue := uint64(0)

	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 55)
	pci.Spawner.On("HandleSend", mock.Anything, mock.Anything, mock.Anything).Return(true)
	pci.Proxy.On("SendToServer", toServer1).Return(nil).Once()
	pci.Proxy.On("SendToClient", toClient1).Return(nil).Once()
	pci.Proxy.On("SendToServer", toServer2).Return(nil).Once()
	pci.Proxy.On("SendToClient", toClient2).Return(nil).Once()
	if !pci.Container.IsAlive() {
		t.Fatalf("Container was closed when created")
	}
	sent := pci.Container.GetBytesSent()
	if sent != expectedValue {
		t.Fatalf("Wrong number of bytes sent, expected %d got %d", expectedValue, sent)
	}
	// CHANNEL
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        toServer1,
	}
	expectedValue += uint64(len(toServer1))
	// Wait for it to get handled - this should take under 50 MS
	time.Sleep(time.Millisecond * 50)
	sent = pci.Container.GetBytesSent()
	if sent != expectedValue {
		t.Fatalf("Wrong number of bytes sent, expected %d got %d", expectedValue, sent)
	}
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: false,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        toClient1,
	}
	expectedValue += uint64(len(toClient1))
	// Wait for it to get handled - this should take under 50 MS
	time.Sleep(time.Millisecond * 50)
	sent = pci.Container.GetBytesSent()
	if sent != expectedValue {
		t.Fatalf("Wrong number of bytes sent, expected %d got %d", expectedValue, sent)
	}
	// ToClient
	err := pci.Container.SendToClient(toClient2)
	if err != nil {
		t.Fatalf("SendToClient failed: %v", err)
	}
	expectedValue += uint64(len(toClient2))
	sent = pci.Container.GetBytesSent()
	if sent != expectedValue {
		t.Fatalf("Wrong number of bytes sent, expected %d got %d", expectedValue, sent)
	}
	// ToServer
	err = pci.Container.SendToServer(toServer2)
	if err != nil {
		t.Fatalf("SendToServer failed: %v", err)
	}
	expectedValue += uint64(len(toServer2))
	sent = pci.Container.GetBytesSent()
	if sent != expectedValue {
		t.Fatalf("Wrong number of bytes sent, expected %d got %d", expectedValue, sent)
	}
}

func TestGetClientAddr(t *testing.T) {
	client := NewMockAddr("TestClient")
	pci := NewProxyContainer(t, client, 3)
	cAddr := pci.Container.GetClientAddr()
	if cAddr.Network() != client.Network() || cAddr.String() != client.String() {
		t.Errorf("Invalid client address, expected %+v got %+v", client, cAddr)
	}
}

func TestGetServerAddr(t *testing.T) {
	server := NewMockAddr("TestServer")
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	pci.Spawner.On("GetServerAddr").Return(server)
	cAddr := pci.Container.GetServerAddr()
	if cAddr.Network() != server.Network() || cAddr.String() != server.String() {
		t.Errorf("Invalid client address, expected %+v got %+v", server, cAddr)
	}
}

func TestGetLastContactTime(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	pci.Spawner.On("HandleSend", mock.Anything, mock.Anything, mock.Anything).Return(true)
	pci.Proxy.On("SendToClient", []byte{0}).Return(nil).Once()
	pci.Proxy.On("SendToServer", []byte{1}).Return(nil).Once()
	pci.Proxy.On("SendToServer", []byte{2}).Return(nil).Once()
	pci.Proxy.On("SendToClient", []byte{3}).Return(nil).Once()
	last := pci.Container.GetLastContactTime()
	if last.Unix() != 0 {
		t.Errorf("Expected last contact time to start at 0, was %d", last.Unix())
	}
	sendTime := time.Now()
	err := pci.Container.SendToClient([]byte{0})
	if err != nil {
		t.Errorf("SendToClient failed: %v", err)
	}
	last = pci.Container.GetLastContactTime()
	if sendTime.Unix() != last.Unix() {
		t.Errorf("Expected '%d' was '%d'", last.Unix(), sendTime.Unix())
	}
	sendTime = time.Now()
	err = pci.Container.SendToServer([]byte{1})
	if err != nil {
		t.Errorf("SendToServer failed: %v", err)
	}
	last = pci.Container.GetLastContactTime()
	if sendTime.Unix() != last.Unix() {
		t.Errorf("Expected '%d' was '%d'", last.Unix(), sendTime.Unix())
	}
	sendTime = time.Now()
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        []byte{2},
	}
	time.Sleep(time.Millisecond * 50)
	last = pci.Container.GetLastContactTime()
	if sendTime.Unix() != last.Unix() {
		t.Errorf("Expected '%d' was '%d'", last.Unix(), sendTime.Unix())
	}
	sendTime = time.Now()
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: false,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        []byte{3},
	}
	time.Sleep(time.Millisecond * 50)
	last = pci.Container.GetLastContactTime()
	if sendTime.Unix() != last.Unix() {
		t.Errorf("Expected '%d' was '%d'", last.Unix(), sendTime.Unix())
	}
}

func TestProxyClose(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	if !pci.Container.IsAlive() {
		t.Errorf("Container was closed when created")
	}
	pci.Container.Close()
	if pci.Container.IsAlive() {
		t.Errorf("Container wasn't closed when context was cancelled")
	}
	cause := context.Cause(pci.ProxyContext)
	if !errors.Is(cause, handler.ErrProxyClosedOk) {
		t.Errorf("Container wasn't closed with ErrProxyClosedOk was %v", cause)
	}
}

func TestProxySendClosed(t *testing.T) {
	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 3)
	if !pci.Container.IsAlive() {
		t.Errorf("Container was closed when created")
	}
	pci.Container.Close()
	if pci.Container.IsAlive() {
		t.Fatalf("Container wasn't closed when context was cancelled")
	}
	err := pci.Container.SendToClient([]byte{0})
	if err == nil {
		t.Errorf("SendToClient didn't fail on closed context")
	}
	err = pci.Container.SendToServer([]byte{0})
	if err == nil {
		t.Errorf("SendToClient didn't fail on closed context")
	}
}

func TestProxySendHandleSendFalse(t *testing.T) {
	dropToServer := []byte("TO_SERVER_1")
	dropToClient := []byte("TO_CLIENT_1")
	expectedSent := uint64(0)

	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 55)
	pci.Spawner.On("HandleSend", dropToServer, mock.Anything, mock.Anything).Return(false)
	pci.Spawner.On("HandleSend", dropToClient, mock.Anything, mock.Anything).Return(false)
	// Logging
	pci.Spawner.On("GetServerAddr").Return(NewMockAddr("TestServer")).Maybe()
	if !pci.Container.IsAlive() {
		t.Fatalf("Container was closed when created")
	}
	// CHANNEL
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        dropToServer,
	}
	sent := pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact := pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        dropToClient,
	}
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	// SENDTO
	pci.Container.SendToClient(dropToClient)
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	pci.Container.SendToServer(dropToServer)
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
}

func TestProxySendReturnError(t *testing.T) {
	toServer := []byte("TO_SERVER_1")
	toClient := []byte("TO_CLIENT_1")
	expectedSent := uint64(0)

	pci := NewProxyContainer(t, NewMockAddr("TestClient"), 55)
	pci.Spawner.On("HandleSend", toServer, mock.Anything, mock.Anything).Return(true)
	pci.Spawner.On("HandleSend", toClient, mock.Anything, mock.Anything).Return(true)
	pci.Proxy.On("SendToClient", toClient).Return(errors.New("test error"))
	pci.Proxy.On("SendToServer", toServer).Return(errors.New("test error"))
	// Logging
	pci.Spawner.On("GetServerAddr").Return(NewMockAddr("TestServer")).Maybe()
	// Deprecated
	pci.Spawner.On("HandleError", mock.Anything, mock.Anything).Maybe()
	if !pci.Container.IsAlive() {
		t.Fatalf("Container was closed when created")
	}
	// CHANNEL (Should just ignore)
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: true,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        toServer,
	}
	sent := pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact := pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	pci.PktChan <- handler.ProxyPacketData{
		Serverbound: false,
		Source:      NewMockAddr("Source"),
		Dest:        NewMockAddr("Dest"),
		Data:        toClient,
	}
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	// SENDTO
	err := pci.Container.SendToClient(toClient)
	if err == nil {
		t.Errorf("Expected error on SendToClient")
	}
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
	err = pci.Container.SendToServer(toServer)
	if err == nil {
		t.Errorf("Expected error on SendToClient")
	}
	sent = pci.Container.GetBytesSent()
	if sent != expectedSent {
		t.Fatalf("Expected GetBytesSent to return %d, returned %d", expectedSent, sent)
	}
	lastContact = pci.Container.GetLastContactTime()
	if lastContact.Unix() != 0 {
		t.Fatalf("Expected GetLastContactTime to return %d, returned %d", 0, sent)
	}
}
