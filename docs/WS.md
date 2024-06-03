# WebSocket info
##### Written for API version 2.1r2
## Getting data from the WebSocket
The API is in a JSON format, every response will include

If 200
```json
{
    "Status": 200,
    "Data": {
        "Type": 1, // Integer
        "Data": {} // Any
    }
}
```

If non 200
```json
{
    "Status": 400, // HTTP Status code, if non 200 the "Data" field is ALWAYS a string
    "Data": "Error"
}
```

The only data sent is WsPacket for now
```go
type CapFlags uint32

const (
	CapFlag_ToServer CapFlags = 1 << 0 // Direction, if set its Serverbound, if not is ClientBound
	CapFlag_Injected CapFlags = 1 << 1 // Is injected
)

type wsPacket struct {
	PktNum  int              // The index of the packet on the WebSocket - Only used for filtering right now
	ProxyId int              // ID of the proxy that this was sent over
	Network string           // Network this packet was sent on, 'tcp' or 'udp'
	Source  string           // Source of this packet
	Dest    string           // Destination of this packet
	Data    []byte           // Packet data. Base64 encoded.
	Flags   handler.CapFlags // Flags, any CapFlag_*, if CapFlag_Inject is set this packet cannot be filtered.
}
```

## Sending data to the websocket
Sending data to the websocket can control various things, the format is always the same
```go
type wsReqType int

const (
    wsReqInject wsReqType = 1
	wsReqClose  wsReqType = 2
	wsReqFilter wsReqType = 3
)

type WsClientMsg struct {
	Type   wsReqType
	Target int    // Inject, Close: Target proxy, Filter: Target proxy
	Data   []byte // Inject: Inject data. Send as base64 encoded.
	Extra  uint64 // Inject: 0, Send to Client. 1, Send to Server. Filter: 0, Drop/Send (0/1)
}
```

### Inject
Requires: inject

Example:
```json
{
    "Type": 1,                  // Required.
    "Target": 1,                // Target proxy, or -1 for all connected.
    "Data": "SGVsbG8gV29ybGQh", // Data, base64 encoded
    "Extra": 3                  // Bitfield, 0 is To Client, 1 is To Server. One of these fields must be set.
}
```

### Close
Requires: close

Example:
```json
{
    "Type": 2,   // Required.
    "Target": 1, // Target proxy, or -1 for all connected.
    "Data": "",  // Unused.
    "Extra": 0   // Unused
}
```

### Filter
Requires: filter

Must be sent with 2 seconds (This will be changed to a config option in the future)

Example:
```json
{
    "Type": 3,   // Required.
    "Target": 1, // Packet number, got from callback.
    "Data": "",  // Unused.
    "Extra": 0   // 0 for drop, 1 for allow.
}
```
