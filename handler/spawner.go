package handler

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
)

type rChan struct {
	Recv   chan PacketChanData
	ctx    context.Context
	cancel context.CancelFunc
}

type mpxInfo struct {
	Name     string
	Addr     net.Addr
	Protocol PxProto
	Listener IProxyListener
}

// Spawns proxies as needed
type ProxySpawner struct {
	currentId          int                     // The current ID of the proxy
	currentIdLock      sync.Mutex              // Lock for currentId
	connections        map[int]IProxyContainer // Connections
	connectionLock     sync.Mutex              // Lock for connections
	totalSent          uint64                  // Total bytes sent
	serverAddr         net.Addr                // Server address
	context            context.Context         // Context for the spawner, this is the parent of all contexts
	contextCancel      context.CancelCauseFunc // Cancel function
	wg                 sync.WaitGroup          // Wait group for handlers
	sendCallback       PacketSendCallback      // Callback for sends
	errorCallback      ProxyErrorCallback      // Callback for errors
	logger             *slog.Logger            // Logger, may be nil
	containerMaker     CreateIProxyContainer   // Create new IProxyContainers
	rcChan             []*rChan
	rcChanLock         sync.Mutex
	callbackCtx        context.Context
	callbackCancel     context.CancelFunc
	totalSentWriteLock sync.Mutex
	mpxLock            sync.Mutex
	mpxs               map[string]mpxInfo
}

// Adds a new proxy, returns the proxies ID or a error if something goes wrong
func (p *ProxySpawner) AddConnection(px IProxy) (IProxyContainer, error) {
	if p.context.Err() != nil {
		p.logger.Error("Attempted to addListener with dead context", "Error", p.context.Err(), "Cause", context.Cause(p.context))
		return nil, p.context.Err()
	}
	// Get a new ID
	p.currentIdLock.Lock()
	thisId := p.currentId
	// Create the container & Initialize the proxy
	p.logger.Debug("Adding new IProxyContainer", "Id", thisId)
	pc, err := p.containerMaker(p, px, thisId)
	if err != nil {
		p.logger.Debug("Failed to create IProxyContainer", "Id", thisId, "Error", err.Error())
		p.currentIdLock.Unlock()
		// Fuck
		p.HandleError(err, nil)
		return nil, err
	}
	p.currentId++
	p.currentIdLock.Unlock()
	// Add the connection
	p.connectionLock.Lock()
	p.connections[thisId] = pc
	p.connectionLock.Unlock()
	p.wg.Add(1)
	return pc, nil
}

func (p *ProxySpawner) addListener(h IProxyListener) {
	if p.context.Err() != nil {
		p.logger.Error("Attempted to addListener with dead context", "Error", p.context.Err(), "Cause", context.Cause(p.context))
		return
	}
	retryCount := 0
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		p.logger.Debug("Running a new listener")
		ctx, cancel := context.WithCancelCause(p.context)
		h(ctx, cancel, p)
		if ctx.Err() == nil {
			p.contextCancel(errors.New("listener didn't cancel context after return"))
			return
		}
		cause := context.Cause(ctx)
		if errors.Is(cause, ErrProxyRetry) {
			// Retry the connection
			if retryCount >= 3 {
				p.logger.Error("Proxy got max retries", "Retries", retryCount, "Error", cause.Error())
				p.HandleError(ErrProxyMaxRetries, nil)
				return
			}
			retryCount++
			// Retry it
			p.logger.Info("Retrying listener", "Retries", retryCount, "Error", cause.Error())
			continue
		} else if errors.Is(cause, ErrProxyClosedOk) {
			p.logger.Info("Proxy closed, closing spawner", "Error", cause.Error())
			p.contextCancel(ErrSpawnerClosedOk)
			return
		} else {
			p.logger.Error("Error when listener closed", "Error", cause.Error())
			p.HandleError(context.Cause(ctx), nil)
			p.contextCancel(cause)
			return
		}
	}
}

func (p *ProxySpawner) RegisterMpx(mpxName string, protocol PxProto, address net.Addr, listener IProxyListener) error {
	p.mpxLock.Lock()
	defer p.mpxLock.Unlock()
	// Verify the mpxName is not in use.
	if _, found := p.mpxs[mpxName]; found {
		return errors.New("mpx name already registered")
	}
	// Check if the address is in use
	for _, v := range p.mpxs {
		// Check if the protocol is already setup
		if v.Addr.String() == address.String() && (v.Protocol == protocol || v.Protocol == PxProtoAll) {
			return errors.New("protocol already in use on address")
		}
	}
	// Add the Mpx
	p.mpxs[mpxName] = mpxInfo{
		Name:     mpxName,
		Addr:     address,
		Protocol: protocol,
		Listener: listener,
	}
	// Run it
	go p.addListener(listener)
	return nil
}

// Callback for all sends
// Calls p.SendCallback if its not nil
func (p *ProxySpawner) HandleSend(data []byte, flags CapFlags, pc IProxyContainer) (shouldSend bool) {
	pktData := PacketChanData{
		Data:    data,
		Flags:   flags,
		ProxyId: pc.GetId(),
	}
	if flags.IsServerbound() {
		pktData.Source = pc.GetClientAddr()
		pktData.Dest = pc.GetServerAddr()
	} else {
		pktData.Source = pc.GetServerAddr()
		pktData.Dest = pc.GetClientAddr()
	}
	p.totalSentWriteLock.Lock()
	p.totalSent += uint64(len(data))
	p.totalSentWriteLock.Unlock()
	if p.sendCallback != nil {
		if p.callbackCtx.Err() != nil {
			p.logger.Debug("Callback context was closed, freeing filter callback", "Cause", context.Cause(p.callbackCtx), "Error", p.callbackCtx.Err().Error())
			p.callbackCancel()
			p.callbackCancel = nil
			p.callbackCtx = nil
			p.sendCallback = nil
		} else {
			p.logger.Debug("Calling sendCallback", "Data", data, "Flags", flags)
			if !p.sendCallback(data, flags, pc) {
				// If we're dropping it we don't need to do anything else.
				return false
			}
		}
	} else {
		p.logger.Debug("No sendCallback, forwarding packet", "Data", data, "Flags", flags)
	}
	// Only send to the channels if we aren't dropping the packet
	p.rcChanLock.Lock()
	for i, v := range p.rcChan {
		if v.ctx.Err() == nil {
			select {
			case v.Recv <- pktData:
			default:
				// No activity - ignore (Maybe remove the chan in the future, but honestly that data leak is on the user)
				p.logger.Warn("Packet data not handled on channel", "Index", i)
			}
		}
	}
	p.rcChanLock.Unlock()
	return true
}

// Error callback
// Logs the error if logger is not nil
// Calls p.errorCallback if its not nil
func (p *ProxySpawner) HandleError(err error, pc IProxyContainer) {
	// TODO: Add ProxyContainer info
	if pc == nil {
		p.logger.Error("HandleError got error in spawner", "Error", err.Error())
	} else {
		p.logger.Error("HandleError got error", "Error", err.Error(), "ProxyContainer", pc.Network())
	}
	if p.errorCallback != nil {
		p.errorCallback(err, pc)
	}
}

// Pruner for all proxies
// Runs every second and removes anything that's IsAlive is false
func (p *ProxySpawner) pruner() {
	p.wg.Add(1)
	ticker := time.NewTicker(time.Second * 1)
	for {

		select {
		case <-ticker.C:
			// Prune connections
			deleteKeys := make([]int, 0)
			p.connectionLock.Lock()
			for k, v := range p.connections {
				// I might add a timeout, but IProxy should build it in.
				if !v.IsAlive() {
					deleteKeys = append(deleteKeys, k)
				}
			}
			if len(deleteKeys) != 0 {
				for _, v := range deleteKeys {
					p.logger.Debug("Removing dead connection", "Id", v)
					delete(p.connections, v)
					p.wg.Done()
				}
			}
			p.connectionLock.Unlock()
			// Prune recvChans
			deleteKeys = make([]int, 0)
			for i, v := range p.rcChan {
				if v.ctx.Err() != nil {
					deleteKeys = append(deleteKeys, i)
				}
			}
			p.rcChanLock.Lock()
			for _, v := range deleteKeys {
				// Is this slow?
				p.logger.Debug("Removing dead recv channel", "Index", v)
				p.rcChan[v] = p.rcChan[len(p.rcChan)-1]
				p.rcChan = p.rcChan[:len(p.rcChan)-1]
			}
			p.rcChanLock.Unlock()
		case <-p.context.Done():
			return
		}
	}
}

// Gets the context of this spawner
func (p *ProxySpawner) GetContext() context.Context {
	return p.context
}

// Get address of the proxy spawner
func (p *ProxySpawner) GetProxyAddr(mpx string) (net.Addr, error) {
	if info, found := p.mpxs[mpx]; found {
		return info.Addr, nil
	}
	return nil, errors.New("mpx not found")
}

func (p *ProxySpawner) GetMpxAddrs() map[string]net.Addr {
	result := make(map[string]net.Addr)
	for k, v := range p.mpxs {
		result[k] = v.Addr
	}
	return result
}

// Get the address of the server
func (p *ProxySpawner) GetServerAddr() net.Addr {
	return p.serverAddr
}

// Gets a proxy by ID, returns a error if its not found
func (p *ProxySpawner) GetProxy(id int) (IProxyContainer, error) {
	p.connectionLock.Lock()
	defer p.connectionLock.Unlock()
	if c, found := p.connections[id]; found {
		return c, nil
	}
	return nil, errors.New("proxy not found")
}

// Get all proxies
func (p *ProxySpawner) GetAllProxies() []IProxyContainer {
	c := make([]IProxyContainer, 0, len(p.connections))
	p.connectionLock.Lock()
	defer p.connectionLock.Unlock()
	for _, v := range p.connections {
		c = append(c, v)
	}
	return c
}

// Closes all the proxies
func (p *ProxySpawner) Close() error {
	p.logger.Debug("Closing spawner")
	for _, v := range p.rcChan {
		v.cancel()
		close(v.Recv)
	}
	p.contextCancel(ErrSpawnerClosedOk)
	doneWg := make(chan bool)
	go func() {
		defer close(doneWg)
		p.wg.Done()
	}()
	select {
	case <-doneWg:
		return nil
	case <-time.After(time.Second):
		p.logger.Debug("Closing spawner timed out")
		return errors.New("timed out closing proxies")
	}
}

// Close a target proxy, returns a error if it doesn't exist.
func (p *ProxySpawner) CloseProxy(id int) error {
	px, err := p.GetProxy(id)
	if err != nil {
		p.logger.Debug("Attempted to close unknown proxy", "Id", id, "Error", err.Error())
		return err
	}
	p.logger.Debug("Closing proxy", "Id", id)
	px.Cancel(ErrProxyClosedOk)
	return nil
}

func (p *ProxySpawner) TrySetFilterCallback(cb PacketSendCallback, ctx context.Context) error {
	if p.callbackCtx != nil {
		if p.callbackCtx.Err() == nil {
			return errors.New("callback already exists")
		}
		p.logger.Debug("Callback context was closed, freeing filter callback", "Cause", context.Cause(p.callbackCtx), "Error", p.callbackCtx.Err().Error())
		p.callbackCancel()
		p.callbackCancel = nil
		p.callbackCtx = nil
		p.sendCallback = nil
	}
	p.logger.Debug("Setting new filter callback")
	p.callbackCtx, p.callbackCancel = context.WithCancel(ctx)
	p.sendCallback = cb
	return nil
}

// Sets the error callback for all proxies
func (p *ProxySpawner) SetErrorCallback(cb ProxyErrorCallback) {
	p.errorCallback = cb
}

// Gets the total number of sent bytes
func (p *ProxySpawner) GetBytesSent() uint64 {
	return p.totalSent
}

// Deprecated: Use GetAllProxies and .SendToClient instead, as it returns errors better.
func (p *ProxySpawner) SendToAllClients(data []byte) error {
	p.logger.Debug("Sending data to all clients", "Data", data)
	var firstErr error
	p.connectionLock.Lock()
	defer p.connectionLock.Unlock()
	for _, v := range p.connections {
		if v.IsAlive() {
			err := v.SendToClient(data)
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Deprecated: Use GetAllProxies and .SendToClient instead, as it returns errors better.
func (p *ProxySpawner) SendToAllServers(data []byte) error {
	p.logger.Debug("Sending data to all servers", "Data", data)
	var firstErr error
	p.connectionLock.Lock()
	defer p.connectionLock.Unlock()
	for _, v := range p.connections {
		if v.IsAlive() {
			err := v.SendToServer(data)
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (p *ProxySpawner) IsAlive() bool {
	return p.context.Err() == nil
}

func (p *ProxySpawner) GetRecvChan(ctx context.Context) (recv <-chan PacketChanData, rCtx context.Context, cancel context.CancelFunc) {
	c, can := context.WithCancel(ctx)
	r := rChan{
		ctx:    c,
		cancel: can,
		Recv:   make(chan PacketChanData),
	}
	p.rcChanLock.Lock()
	p.rcChan = append(p.rcChan, &r)
	p.rcChanLock.Unlock()
	return r.Recv, r.ctx, r.cancel
}

// Creates a new proxy spawner
// Uses default container (NewProxyContainer)
// logger may be nil, at least one listener must exist.
func NewProxySpawner(server net.Addr, ctx context.Context) (*ProxySpawner, error) {
	return NewProxySpawnerWithContainer(server, NewProxyContainer, ctx)
}

// Creates a new proxy spawner
// logger may be nil, at least one listener must exist.
func NewProxySpawnerWithContainer(server net.Addr, containerMaker CreateIProxyContainer, ctx context.Context) (*ProxySpawner, error) {
	psContext, cancel := context.WithCancelCause(ctx)
	ps := &ProxySpawner{
		currentId:          0,
		currentIdLock:      sync.Mutex{},
		connections:        make(map[int]IProxyContainer),
		connectionLock:     sync.Mutex{},
		totalSent:          0,
		serverAddr:         server,
		context:            psContext,
		contextCancel:      cancel,
		wg:                 sync.WaitGroup{},
		sendCallback:       nil,
		containerMaker:     containerMaker,
		errorCallback:      nil,
		logger:             slog.Default(),
		rcChan:             make([]*rChan, 0),
		rcChanLock:         sync.Mutex{},
		callbackCtx:        nil,
		callbackCancel:     nil,
		totalSentWriteLock: sync.Mutex{},
		mpxLock:            sync.Mutex{},
		mpxs:               make(map[string]mpxInfo),
	}
	if ps.context.Err() != nil {
		err := context.Cause(ps.context)
		ps.logger.Warn("Listener failed during creation of spawner", "Error", err.Error())
		ps.contextCancel(err)
		return nil, err
	}
	ps.logger.Debug("Starting pruner")
	go ps.pruner()
	return ps, nil
}
