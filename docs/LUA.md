# Lua

Lua bindings for EzProxy

This is in *very* early testing, and the API **will** change.

Honestly, LUA is much worse then the websocket - I'd use the websocket.


## Usage Guide

EzProxy must be compiled with the `lua_bindings` build tag (`go build -tags lua_bindings .`)

In your config file the `Lua` section should look like

```yaml
# Trimmed 
Lua:
  Enable: true
  Path: my_script.lua
  Mode: # 'main' or 'callback'
```

Using the 'main' mode will execute `EzpMain`, using callback mode will call various callbacks when things happen, see below for more info.

### 'main' mode
The function `EzpMain` in your script will be executed, it takes a [EzpSpawner](#ezpspawner) table as a argument.

Example:
```lua
function EzpMain(ezp)
    local server = ezp.get_server_address()
    local proxy = ezp.get_proxy_address()
    while true do 
        lprint("Alive=%s", ezp.is_alive())
        lprint("Server=%s | Proxy=%s", server, proxy)
        lprint("BytesSent=%d", ezp.get_bytes_sent())
        lprint("Connected=%d", ezp.get_proxy_count())
        log(LEVEL_INFO, "Hello Lua")
        sleep(10000)
    end
end
```

### 'callback' mode
An important note is that because LUA is not multithreaded packets not handled will be ignored, this behavior is different and will be documented.

## `EzpOnPacket(PacketData, EzpSpawner) -> bool`
Cannot exist alongside [EzpFilter](#ezpfilterpacketdata-ezpspawner---bool)

Takes [PacketData](#packetdata) and [EzpSpawner](#ezpspawner) as arguments, called every time a packet is received.

If another packet comes in while you are handling a previous one it will be skipped.

If `true` is returned the callback will be uninstalled and the LUA script will be terminated.

## `EzpFilter(PacketData, EzpSpawner) -> bool`
Cannot exist alongside [EzpOnPacket](#ezponpacketpacketdata-ezpspawner---bool)

Takes [PacketData](#packetdata) and [EzpSpawner](#ezpspawner) as arguments, called every time a packet is received.

If another packet comes in while you are handling a previous one you will have a configured amount of time (500 MS by default) to handle the packet, before the default action (By default the packet is dropped) happens and the packet is ignored.

If `true` is returned the packet is allowed, if `false` the packet will be dropped.

Injected packets ignore filter requests and always go through, its important you don't repeat actions because of packets you just injected, for instance

```lua
function EzpFilter(ezp, pkt)
    -- Inject some data
    ezp.inject_to_server(pkt.proxy_id, "Hello from Lua!")
    -- Drop the original packet
    return false
end
```

May produce a stack overflow because each injected packet will be duplicated and at some point LUA will just crash.

Adding a injection guard will fix this

```lua
function EzpFilter(ezp, pkt)
    if (pkt.injected) then 
        -- This return value is discarded
        return true
    end
    -- Inject some data
    ezp.inject_to_server(pkt.proxy_id, "Hello from Lua!")
    -- Drop the original packet
    return false
end
```

# Programming Reference

## Globals
All functions will throw errors if a incorrect number of arguments is provided, or the arguments are of invalid types.

### Constants
Levels to using with `log`, these values are
* LEVEL_DEBUG
* LEVEL_INFO
* LEVEL_WARN
* LEVEL_ERROR
  
### `log(level: int, message: string) -> nil`
Logs a message using the Go logger, outputting to the `log` file, the log will include basic info about the running lua file.

Raises a error if the `level` was not one of the `LEVEL_*` constants

`level`: One of the `LEVEL_*` constants.

`message`: The message to print

### `sleep(ms: int) -> nil`
Sleeps the script for the given time in MS

Raises a error if `ms` is negative.

`ms`: The number of milliseconds to sleep for

## EzpSpawner
### `inject_to_*(target_id: int, data: string) -> nil`
`inject_to_server`, `inject_to_client`, `inject_to_both`

Injects data through the target proxy

Raises a error if `target_id` was not `-1` and not a valid proxy.

`target_id`: Id to inject to, or `-1` to inject to all connected proxies.

`data`: Data to inject

### `is_alive() -> bool`
Returns if the spawner is alive.

### `get_*_address() -> string`
`get_server_address`, `get_client_address`

Gets the requested address as \<IP\>:\<PORT\>

Example: `127.0.0.1:1234`

### `get_packets(callback: (EzpSpawner, PacketData) -> bool) -> nil`
Waits to get packets, blocking current execution.

Calls the `callback` every time a packet is received. If the callback returns `true` the callback is removed and this call stops blocking.

Raises a error if `callback` raises a error.

`callback`: A function that takes a `EzpSpawner` and `PacketData` and returns a bool indicating if this function should stop waiting for packets.

Example
```lua
function callback(ezp, pData) 
    lprint("{%s->%s}", pData.source, pData.dest)
    -- Don't uninstall
    return false
end
```

### `close(id: int) -> nil`
Closes a target.

If `id` is `-1` all proxies will be closed.

If `id` is `-2` the spawner will be closed along with all proxies, this will terminate this script as well.

Otherwise the Id is taken as a proxy id

Raises a error if `id` is not `-2`, `-1` or a valid proxy ID

`id`: A number that is either `-2` to close the spawner, `-1` to close all proxies or a valid proxy ID

### `get_bytes_sent() -> int`
Gets the number of bytes sent over all proxies in this run time

### `get_proxy_count() -> int`
Get currently connected proxy count

### `get_proxy(id: int) -> EzProxy`
Get a target [proxy](#ezproxy)

Raises a error if `id` is not a valid proxy.

`id`: A valid proxy ID

## EzProxy
### `is_alive() -> bool` 
Returns if this proxy is alive

### `send_to_*(data: string) -> nil`
`send_to_client`, `send_to_server`

Sends data to target.

`data`: Data to send

### `get_id() -> int`
Get the ID of this proxy

### `get_network() -> string`
Get network type of this proxy

Examples: `tcp`, `udp`

### `get_*_addr() -> string`
`get_client_addr`,  `get_server_addr`

Get address as IP:PORT

Example: 127.0.0.1:1234

### `get_bytes_sent() -> int`
Get the number of bytes sent through this proxy

###  `get_last_contact() -> int`
Get the number of milliseconds since the last data sent on this proxy

## PacketData
### `flags: int`
`CapFlags_*` bitfield

```go
// Capture flags
type CapFlags uint32

const (
	CapFlag_ToServer CapFlags = 1 << 0 // Direction, if set its Serverbound, if not is ClientBound
	CapFlag_Injected CapFlags = 1 << 1 // Is injected
)
```

### `serverbound: bool`
Checks for the `CapFlag_ToServer` bit

### `injected: bool`
Checks for the `CapFlag_Injected` bit

### `source: string`
IP:PORT source

Example: 127.0.0.1:1234

### `dest: string`
IP:PORT destination

### `proxy_id: int`
Id of the proxy this was sent on

### `data: string`
Packet data