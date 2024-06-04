package handler

import (
	"context"
	"net"
	"time"
)

// Called before a packet is sent
// data  : Bytes to be sent
// flags : Send flags
// proxy : Proxy interface
// return: Should this packet be sent
type PacketSendCallback func(data []byte, flags CapFlags, proxy IProxyContainer) (shouldSend bool)

// p may be nil iff the Spawner threw the error
type ProxyErrorCallback func(err error, pc IProxyContainer)

// Received packet
type ProxyPacketData struct {
	Serverbound bool     // Is serverbound
	Source      net.Addr // Source address
	Dest        net.Addr // Dest address
	Data        []byte   // Data
}

type PacketChanData struct {
	Flags   CapFlags
	Source  net.Addr
	Dest    net.Addr
	Data    []byte
	ProxyId int
}

// Add a new connection with .AddConnection in the ProxySpawner
// .AddConnection must be called when a new connection is created or it will not be added to the list.
// If the context is not canceled with 'cancel' or already cancelled and this function exits the spawner context will be cancelled for you, but this is not recommended.
//
// ctx   : Context of this listener, should be aborted when the context dies.
//
// cancel: Cancel the context with a reason, if the reason is ErrProxyClosed or ErrSpawnerClosed no error will be logged.
//
// ca    : Connection adder for adding connections & getting limited information about the spawner.
type IProxyListener func(ctx context.Context, cancel context.CancelCauseFunc, ca IConnectionAdder)

// A container for a IProxy
type IProxyContainer interface {
	IsAlive() bool                     // Returns true if the proxy is currently alive
	Cancel(cause error)                // Cancels the container and proxy
	SendToClient(data []byte) error    // Sends data to the client, this counts as a injection.
	SendToServer(data []byte) error    // Sends data to the server, this counts as a injection.
	GetId() int                        // Gets the ID of this proxy
	Network() string                   // Gets the network the proxy is now
	GetServerAddr() net.Addr           // Gets the address of the server
	GetClientAddr() net.Addr           // Gets the address of the client
	GetBytesSent() uint64              // Gets the total number of bytes sent
	LastContactTimeAgo() time.Duration // Gets the last time data was sent or received from this proxy
}

// Creates a new proxy container
//
// parent: Spawner this container is being spawned from
//
// px    : Proxy being added
//
// id    : ID of this proxy
//
// returns: Container or a error
type CreateIProxyContainer func(parent IProxySpawner, px IProxy, id int) (IProxyContainer, error)

// Proxy implementation
type IProxy interface {
	Init(pktChan chan<- ProxyPacketData, ctx context.Context, cancel context.CancelCauseFunc) error // Initialize the proxy with Container info, do not allow the proxy to run until this is called.
	SendToClient(data []byte) error                                                                 // Send data to client
	SendToServer(data []byte) error                                                                 // Send data to server
	GetClientAddr() net.Addr                                                                        // Gets the client
	Network() string                                                                                // Gets the network we are on
}

// Proxy spawner
type IProxySpawner interface {
	IConnectionAdder
	GetContext() context.Context                                                                                   // Gets the context the spawner is using
	GetAllProxies() []IProxyContainer                                                                              // Gets all proxies currently alive
	Close() error                                                                                                  // Closes the spawner and all proxies
	CloseProxy(id int) error                                                                                       // Closes the target proxy if it exists, if not a error is returned
	TrySetFilterCallback(cb PacketSendCallback, ctx context.Context) error                                         // Attempt to set the filter callback, if one is already set the context is cancelled.
	SetErrorCallback(cb ProxyErrorCallback)                                                                        // Sets the error callback
	GetBytesSent() uint64                                                                                          // Gets the number of bytes sent from all proxies, dead and alive.
	SendToAllClients(data []byte) error                                                                            // Deprecated: Use GetAllProxies and .SendToClient instead, as it returns errors better.
	SendToAllServers(data []byte) error                                                                            // Deprecated: Use GetAllProxies and .SendToServer instead, as it returns errors better.
	IsAlive() bool                                                                                                 // Checks if the spawner is alive
	GetRecvChan(ctx context.Context) (recv <-chan PacketChanData, rCtx context.Context, cancel context.CancelFunc) // Get a unique channel to handle get packets, this channel will be closed when the context is closed, this is a unbuffered channel and will not block if packets are not read.
	// Deprecated
	HandleSend(data []byte, flags CapFlags, proxy IProxyContainer) (shouldSend bool) // Handles a packet being sent
	HandleError(err error, pc IProxyContainer)                                       //  Handles a error being thrown, if pc is nil the error is in IProxySpawner
}

type IConnectionAdder interface {
	GetProxy(id int) (IProxyContainer, error)         // Gets a proxy by ID, if the proxy is not found a error is returned.
	GetProxyAddr() net.Addr                           // Gets the address of the proxy
	GetServerAddr() net.Addr                          // Gets the address of the server
	AddConnection(px IProxy) (IProxyContainer, error) // Adds a connection to the spawner
}
