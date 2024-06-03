# EzProxy bindings


## Support

| Language | Version | API Support | WebSocket           | Tested  |
|----------|---------|-------------|---------------------|---------|
| Python   | 2.1r1   | Full        | Full                | Yes     |
| C#       | 2.1r1   | Full        | Listening Only      | No      |
| C        |         |             |                     |         |
| C++      |         |             |                     |         |
| Go       |         |             |                     |         |


## How-to
###### As of version 2.1r1

### HTTP API
All requests require the `key` query parameter with a valid API key, if you don't have a valid permission a error will be returned.

#### Base response
```go
type BaseResponse struct {
    Status int // If status is non 200 'Data' is a error message
    Data any
}
```

Examples:
```json
{
    "Status": 200,
    "Data": {}
}
```
```json
{
    "Status": 404,
    "Data": "Resource not found"
}
```


#### /api/1/status
Method: GET
<br>Auth: AuthCanCheckStatus

Get status of the spawner

```go
type HandlerStatus struct {
	ConnectionCount int    // Number of connections
	Alive           bool   // Is the handler alive
	BytesSent       uint64 // Number of bytes sent
	ProxyAddress    string // Proxy address (IP):(PORT)
	ServerAddress   string // Server address (IP):(PORT)
}
```

```json
{
    "ConnectionCount": 5,
    "Alive": true,
    "BytesSent": 2000,
    "ProxyAddress": "192.168.0.1:5050",
    "ServerAddress": "192.168.0.1:1234"
}
```

#### /api/1/proxies
Method: GET
<br>Auth: AuthCanCheckStatus

Gets info about all running proxies

```go
type ProxyStatus struct {
	Id             int    // Proxy Id
	Alive          bool   // Is the client alive
	Address        string // (IP):(Port) of this client
	Network        string // Network this proxy is connected on
	BytesSent      uint64 // Number of bytes sent
	LastContactAgo int64  // last contact ago in MS
}
```

```json
[
    {
        "Id": 1,
        "Alive": true,
        "Address": "192.168.0.1:5555",
        "Network": "tcp",
        "BytesSent": 5555,
        "LastContactAgo": 1230
    },
    {
        "Id": 2,
        "Alive": true,
        "Address": "192.168.0.1:6666",
        "Network": "tcp",
        "BytesSent": 555534,
        "LastContactAgo": 55
    }
]
```

#### /api/1/inject
Method: POST
<br>Auth: AuthCanInject

Injects a packet into a proxy

`ToClient` or `ToServer` must be true and `Id` must exist.

```go
type InjectData struct {
	Id       int    // Proxy ID, set to -1 for all
	Data     []byte // Inject data
	ToClient bool   // Send to client(s)
	ToServer bool   // Send to server(s)
}
```

```json
{
    "Id": -1,
    "Data": "SGVsbG8gV29ybGQh", // Must be Base64
    "ToClient": true, 
    "ToServer": true
}
```

#### /api/1/newkey
Method: GET
<br>Auth: AuthCanMakeKeys

Create a new key with a key

You can duplicate any permission you have, minus `AuthCanMakeKeys`, you need `AuthCanDuplicateKeys` to duplicae that.

Set the query value `perms`to a integer of a bit field of permissions
```go
AuthCanCheckStatus   AuthCodes = 1 << 0 // /api/1/status, /api/1/proxies
	AuthCanClose         AuthCodes = 1 << 1 // api/1/close (Closing proxies) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanUseWebsocket  AuthCodes = 1 << 2 // /api/1/socket (Listening, not injecting or filtering)
	AuthCanFilter        AuthCodes = 1 << 3 // /api/1/socket (Filtering, requires authCanUseWebsocket)
	AuthCanInject        AuthCodes = 1 << 4 // /api/1/inject (Injecting) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanMakeKeys      AuthCodes = 1 << 5 // /api/1/key (Creating new keys) You can still only create keys with permissions matching your own, minus this one
	AuthCanDuplicateKeys AuthCodes = 1 << 6 // /api/1/key (Creating new keys) Can create keys matching these permissions including AuthCanMakeKeys

	AuthAll            AuthCodes = 0xfffffffffffffff // All auth values
	AuthAllButMakeKeys AuthCodes = 0xfffffffffffffdf // All auth values but make keys
```

Response
```go
type NewKeyData struct {
	Key   string // A hex string of your key
	Perms int    // Permission bitfield
}
```

```json
{
    "Key": "123DA",
    "Perms": 3
}
```

#### /api/1/keyinfo
Method: GET
<br>Auth: Any valid key

Gets the permission values of a key.

```go
type authValue struct {
	Value            int  // Permission value of this key
	CanCheckStatus   bool // AuthCanCheckStatus
	CanClose         bool // AuthCanClose
	CanUseWebsocket  bool // AuthCanUseWebsocket
	CanFilter        bool // AuthCanFilter
	CanInject        bool // AuthCanInject
	CanMakeKeys      bool // AuthCanMakeKeys
	CanDuplicateKeys bool // AuthCanDuplicateKeys
	Admin            bool // AuthAll
}
```

```json
{
    "Value": 1,
    "CanCheckStatus": true,
    "CanClose": false,
    "CanUseWebsocket": false,
    "CanFilter": false,
    "CanInject": false,
    "CanMakeKeys": false,
    "CanDuplicateKeys": false,
    "Admin": false,
}
```

#### /api/2/socket
Method: GET
<br>Auth: AuthCanUseWebsocket

Queries:
* close: Allows closing proxies, requires `AuthCanClose`
* inject: Allows injecting into proxies, requires `AuthCanInject`
* filter: Allows filtering of packets, requires `AuthCanFilter`
* default: Only used if `filter` is true, must either be `drop` or `allow`, defining the default action if a packet times out.
* network: Filter what packets will be sent on the websocket callback

Make request with the ws:// schema.

On success a websocket is opened.


### WebSocket
#### Base Message

From server:
```go
type wsServerTypes int

const (
	wsServerError  wsServerTypes = -1 // Should never be received, used as a internal nil
	wsServerPacket wsServerTypes = 1  // A packet, type is wsPacket
)

type wsServerMsg struct {
	Type wsServerTypes
	Data any
}

type BaseResponse struct {
	Status int
	Data   interface{} // If Status != 200 this is a error string
}

```

```json
{
    "Status": 200,
    "Data": {
        "Type": 1,
        "Data": // wsPacket
    }
}
```

```json
{
    "Status": 400,
    "Data": "Error"
}
```


From Client
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
	Data   []byte // Inject: Inject data
	Extra  uint64 // Inject: 0, Send to Client. 1, Send to Server. Filter: 0, Drop/Send (0/1)
}

```

#### wsServerPacket
Sent from the server to the client, info about a packet being sent

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
	Data    []byte           // Packet data (Base64)
	Flags   handler.CapFlags // Flags, any CapFlag_*, if CapFlag_Inject is set this packet cannot be filtered.
}
```

#### Inject
Auth: `inject`

Inject packets

`Type` must be 1
<br>`Target` is the target proxy, or `-1` for all
<br>`Data` is the data to inject in Base64 format
<br>`Extra` is a bitfield where bit 0 is set if the packet should be sent to client and bit 1 for server.

```json
{
    "Type": 1,    // 1 = wsReqInject
    "Target": -1, // Inject target, -1 is all
    "Data": "",   // Base64 inject data
    "Extra": 1    // Send to client, set to 2 to server and 3 for both
}
```

#### Filter
Auth: `filter`

This must sent within 500ms of the packet being received to prevent the default action from happening

`Type` must be 2
<br>`Target` is the `PktNum` from `wsPacket`
<br>`Data` is unused
<br>`Extra` is set to `0` to drop and `1` to allow, any non zero value will allow but this may change in the future.

```json
{
    "Type": 2,    // 2= wsReqFilter
    "Target": 2,  // PktNum
    "Data": null, // Unused
    "Extra": 0    // Drop, 1 is send
}
```

##### Close
Auth: `close`

`Type` must be 3
<br>`Target` is id of the proxy, or `-1` for all proxies
<br>`Data` is unused
<br>`Extra` is unused

```json
{
    "Type": 3,    // 2= wsReqFilter
    "Target": 2,  // Proxy ID or -1 for all
    "Data": null, // Unused
    "Extra": 0    // Unused
}
```
