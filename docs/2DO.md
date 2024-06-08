# EzProxy Todo list
## General
- [ ] Unit testing (Coverage >= 90%)
- [ ] Integration testing of UDP & TCP 
    - [ ] Disconnect & Reconnect
    - [ ] Multiple clients
## Proxies
- [ ] Changing clients & server while live
<br>This could be something like
``` go
struct IProxy {
    ChangeClient(newAddr net.Addr) error
    ChangeServer(newAddr net.Addr) error
}
```
- [ ] Multi clients on one proxy 
<br>Something like 
```go
struct IProxy {
    AddClient(c net.Conn) error
}
```
type thing, to allow for multiple clients sending data over the same proxy & all getting data, maybe have some kinda `flags` field for what we should do too.

Only the *original* connections is able to filter packets, that or any connection can, but once one is its locked.
## Handler / Spawner
## API
- [ ] API methods to change Client, Server & Add a client
- [X] Allow disabling need to auth keys
- [ ] Change config & restart spawner.
## Bindings
- [X] Attempt to improve C# speed 
<br>Accidentally left a bit of sync code in where was very important it was *not* sync.
- [ ] C 
- [ ] Go
<br>This probably should have been second huh.
- [X] Lua (Embedded scripting)
## Spawner
- [ ] Remove `HandleSend` or replace it, its sorta a goofy way to do this that maybe would be better with a channel that reads & sends data if its allowed, thats seems way easier & safer.
  
## Interesting
- [ ] Use raw sockets (On UNIX use `socket(AF_PACKET, RAW_SOCKET))` for L2 or `socket(AF_INET, RAW_SOCKET)` for L3) on windows use [npcap](https://npcap.com/guide/wpcap/pcap_inject.html) `pcap_inject`<
<br>Its hard to state how much easier this is on Unix vs windows - The one issues is I don't know if the OS net stack will actually let us do this, usually it just sends a `RST` or otherwise bounces the packet if it didn't get told the port is open
I suppose in theory we could just filter any RST we didn't inject - Honestly this seems less and less like a `EzProxy` issue, but it could be a niche proxy you could use if you wanted to, I think you'd need to configure it externally though because theres no way to really detect 
where the packet is supposed to go.

Actually heres the better implementation 

1. You connected with TCP (Or UDP maybe?) and configure the connections with some sorta protobuf/JSON that tells us how to handle things
   * What layers do you want to control (L2, L3 or L4)
2. The client sends a packet, this is a normal TCP packet but the payload is an entire packet from whatever layer they selected and up, we forward it to server
3. If we need to add layers we do that now
4. Send it, respond with replies from server
5. When the TCP connection dies close the raw socket

The issue I think is going to be around step 4, because they OS is going to drop the reply as we haven't actually opened the socket, I dunno how to really prevent that, in the past i've used `iptables` rules, but thats no on windows, also I don't know how much I want to require EzProxy to use `iptables`, maybe we only support L3 and up for now?/ 
