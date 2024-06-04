* [ ] Testing
* [ ] Document & restrict visibility where possible (For real, why are so many of these things public when there is no legit use for it?)
    * [X] API
    * [ ] Handler
    * [ ] Proxy
    * [ ] Integration
    * [ ] Mocks (Probably just disable godocs for this)
* [ ] Make sure public methods are truly "safe" (Input validation, Documentation, Edge cases checked with & errors documented.)
* [X] Change from callbacks to a recv channel slice that we clone, and removed 'closed' channels, it might need to either pass 2 channels, or for saying the channel is dead, or a linked context we can cancel, that or just accept multiple callbacks & return some sorta callback function to remove it, maybe a waitgroup?

For data transmission
  * [X] Channel
  * [ ] Multiple callbacks
  
For removing the callback
  * [X] Context 
  * [ ] Mutex
  * [ ] Secondary 'signal' channel
  * [ ] Remove with index
  * [ ] Remove with ID
  * [ ] For callbacks, remove by returning a certain value
  * [ ] Waitgroup
   
Heres The issue, we need only one callback able to filter too, so maybe we split it into (I think this is the better idea.)
1. 'Old style' of callback for a single 'filtering enabled' callback
2. One of the above as a read only callback.
   
It could also be a secondary channel we send filter results on


```go
// This one seems better & easier to implement.
func GetRecvChan() (recv <-chan PacketData, ctx context.Context, cancel context.CancelFunc)
// This one is just as annoying & seems worse in every possible way.
func GetRecvChan() (recv <-chan PacketData, close <-chan struct{})
```

* [X] Make sure the API respects when its filtering has been taken over

For instance, when LUA takes over the API should respond that filtering is not its thing anymore and instead use a `RecvChan` instance, maybe the API should use `RecvChan` unless it requires filtering, in which case it attempts to take the callback, but because we already have one it would be locked (Maybe pass a `Context` we can close it when we no longer need filtering.)

* [X] Fix 'default' actions and WebSocket crashes resulting in DOS attacks. (I think this is fixed)
    * Keep some kind of function to keep this action / close the spawner incase thats desirable.
    * By default when the WebSocket dies all data should just go through.
* [X] Either the WebSocket or the Python bindings seem to be doing the wrong thing with filtering packets... fix that.
<br>It was the Python bindings, the filter types were wrong in const.py (Reversed) and in `EzProxyWs._listen` had a flaw
```python
if self._can_filter and sv.Data.Flags != CapFlag_Injected == 0:
    self.filter(sv.Data.PktNum, r)
```
Makes actually no sense, I was so exhausted I messed it up, changed to 
```python
if self._can_filter and sv.Data.Flags & CapFlag_Injected == 0:
    self.filter(sv.Data.PktNum, r)
```
Resolves this issue
* [X] Logging propagation changes
<br>Instead of passing slog.Logger just use `slog.SetDefault`
<br>Or use `logger.With` type stuff [HowTo](https://betterstack.com/community/guides/logging/logging-in-go/#creating-and-using-child-loggers)
* [ ] NAT Types?
* [ ] Maybe add a API change config & restart type thing?
* [ ] API Change server address & proxy address.
<br>A new method in the `IProxy` interface would be needed, like `UpdateConfig` or `UpdateServer` thing
```go
type IProxy interface {
    UpdateServer(newServer net.Addr) error
}
```
that or we could just recreate all the `IProxy` instances, but that sorta seems like a pain, also you could update the server without killing the client as there 2 different connections.
* [ ] Switch clients on the fly 
<br>Using either Websockets something like
```go
type IProxy interface {
    ChangeClient(newAddr net.Adder) error
}
```
could allow for changing client addresses on the fly (Killing the old client), but keeping the connection alive, we could also do a TCP 'pool' thing where multiple TCP connections are done in a NAT way, thats kinda interesting I guess.
* [X] C# bindings speed up
* [X] Allow disabling API
## Possible
* [X] Use [go-lua](https://github.com/Shopify/go-lua) to allow for native scripting, honestly though I think this is a pretty useless idea as WebSockets work well, but hey maybe in more limited environments this would be nice?