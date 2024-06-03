# Changes in 'MultiPx'
## MAJOR CHANGE: MultiPx
Proxies now can now register different proxy IPs for different types, multiple of the same protocol cannot exist at the same time

They will still connect to the same server.

This allows a socks esc UDP over TCP and a plain TCP proxy to exist in the same instance (On different addresses)

The current name of these are 'MultiPx' or 'MultiPxs'

Names are prefixed by Mpx

A Mpx can hold one proxy of each protocol for instance a Mpx can handle Plain TCP & Plain UDP, but not Plain TCP & UDP over TCP as they are both TCP protocols.

Proxies not require a MpxName associated with them, this should be formatted as pascal casing, for instance TcpPlain, UdpPlain

## CONFIG: Mpx
ProxyAddress has been replaced with `Mpx`

```yaml
Mpx:
  MyMpx:
	Address: "Target"
	Port: 1234
  MyMpx2:
	Address: "Target2"
	Port: 234
```

## MAIN: Register Mpx
When you add a new proxy you must also register it in `mpx.go`

```go
// mpx.go

func GetMpxInfo(name string) (handler.PxProto, handler.IProxyListener, error) {
	switch name {
	case "TcpPlain":
		return handler.PxProtoTcp, proxy.TcpListener, nil
	case "UdpPlain":
		return handler.PxProtoUdp, proxy.UdpListener, nil
	case "UdpOverTcp":
		return handler.PxProtoTcp, proxy.UdpOverTcpListener, nil
	default:
		return 0, nil, errors.New("mpx not found")
	}
}
```

to add a new Mpx handler go 

```go
// mpx.go

switch name {
	// ...
	case "MyMpx":
		return handler.PxProtoBoth, proxy.MyMpxListener, nil
}
```

You need to pick the protocol it will use you can use 
```go
// handler/interfaces.go

const (
	PxProtoTcp PxProto = iota // Locks other Tcp protos from existing on a address
	PxProtoUdp                // Locks UDP
	PxProtoAll                // Locks everything.
)
```

## API: /api/1/handler
ServerAddress has been changed to MpxAddresses
```go
// ### OLD ###
// Status of a handler, used for /api/1/handler
type handlerStatus struct {
	ConnectionCount int    // Number of connections
	Alive           bool   // Is the handler alive
	BytesSent       uint64 // Number of bytes sent
	ProxyAddress    string // Proxy address (IP):(PORT)
	ServerAddress   string // Server address (IP):(PORT)
}
// ## NEW ##
// Status of a handler, used for /api/1/handler
type handlerStatus struct {
	ConnectionCount int               // Number of connections
	Alive           bool              // Is the handler alive
	BytesSent       uint64            // Number of bytes sent
	MpxAddresses    map[string]string // MpxName -> Proxy address (IP):(PORT)
	ServerAddress   string            // Server address (IP):(PORT)
}
```

# SPELLCHECK
I guess my spellcheck extension broke or something because I really thought It was covering for me. Spellcheck is actual brain rot and I just cant spell anymore I guess.