# API info 
##### Written for API version 2.1r2
The API is in a JSON format, every response will include
```json
{
    "Status": 200, // HTTP Status code, if non 200 the "Data" field is ALWAYS a string
    "Data": {}     // Any
}
```

Endpoints are formatted as 

/api/(VERSION)/(ENDPOINT)

If enabled, authentication is done with a 'key' query parameter. For instance

`myhost/api/1/myendpoint?key=BEEF`

**Permission enum**
```go
const (
	AuthCanCheckStatus   AuthCodes = 1 << 0 // /api/1/status, /api/1/proxies
	AuthCanClose         AuthCodes = 1 << 1 // api/1/close (Closing proxies) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanUseWebsocket  AuthCodes = 1 << 2 // /api/1/socket (Listening, not injecting or filtering)
	AuthCanFilter        AuthCodes = 1 << 3 // /api/1/socket (Filtering, requires authCanUseWebsocket)
	AuthCanInject        AuthCodes = 1 << 4 // /api/1/inject (Injecting) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanMakeKeys      AuthCodes = 1 << 5 // /api/1/key (Creating new keys) You can still only create keys with permissions matching your own, minus this one
	AuthCanDuplicateKeys AuthCodes = 1 << 6 // /api/1/key (Creating new keys) Can create keys matching these permissions including AuthCanMakeKeys

	AuthAll            AuthCodes = 0xfffffffffffffff // All auth values
	AuthAllButMakeKeys AuthCodes = 0xfffffffffffffdf // All auth values but make keys
)
```

### Status
/api/1/status
<br>Gets the status of the proxy spawner
<br>Method: `GET`
<br>Requires `AuthCanCheckStatus`

```go
type HandlerStatus struct {
	ConnectionCount int    // Number of connections
	Alive           bool   // Is the handler alive
	BytesSent       uint64 // Number of bytes sent
	ProxyAddress    string // Proxy address (IP):(PORT)
	ServerAddress   string // Server address (IP):(PORT)
}
```

### Proxies
/api/1/proxies
<br>Gets status of connected proxies
<br>Method: `GET`
<br>Requires `AuthCanCheckStatus`

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

### Inject
/api/1/inject
<br>Inject data into a proxy
<br>Method: `POST`
<br>Requires `AuthCanInject`

Empty response.

**POST DATA**
```go
type InjectData struct {
	Id       int    // Proxy ID, set to -1 for all
	Data     []byte // Inject data
	ToClient bool   // Send to client(s)
	ToServer bool   // Send to server(s)
}
```


### New key
/api/1/newkey
<br>Creates a new key
<br>Method: `GET`
<br>Requires `AuthCanMakeKeys`
<br>Query parameters are 
* perms `uint64`: Permission bitfield (See above)

You must have all the permissions you are attempting to create with to create a key with those, the only exception is `AuthCanMakeKeys`, to duplicae that value you need `AuthCanDuplicateKeys`.

```go
type NewKeyData struct {
	Key   string // Key value
	Perms int    // Permission bitfield
}
```

### Key Info
/api/1/keyinfo
<br>Get info about a key
<br>Method: `GET`
<br>Query parameters are 
* key `uint64`: Key to inspect.

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

### Socket
/api/2/socket
<br>Opens a new websocket connection.
<br>Method: `GET`, must be made with a `ws://` or `wss://` extension.
<br>Requires `AuthCanUseWebsocket`
<br>Query parameters
* close: Has no value, must have `AuthCanClose`, allows closing proxies via websocket
* inject: Has no value, must have `AuthCanInject`, allows injecting data via websocket
* filter: Has no value, must have `AuthCanFilter`, allows filtering via websocket, there cannot be more than 1 filterer connected at any given time.
* default: 'drop' or 'allow, only used if 'filter' is set, defines the default action if a packet is not filtered in time, if 'drop' the packet will be dropped, if 'allow' it will be allowed, by default packets are allowed.
* network: Must be '', 'tcp' or 'udp', only sends matching network data through the WebSocket, by default it is '', which means any.

If all of this is ok a websocket will be opened (See WS.md for more info)