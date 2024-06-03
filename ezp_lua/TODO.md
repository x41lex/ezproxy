# LUA TODO
* [ ] luaLog should log the lua path & line number it was executed on
* [ ] I'd like so type of intellisense, but I don't think its going to happen.
* [ ] A way to reload files would be cool.
## Implement most things
* [ ] IProxySpawner
  * [X] GetProxy
  * [X] GetProxyAddr
  * [X] GetServerAddr
  * [ ] GetAllProxies
  * [X] Close
  * [X] CloseProxy
  * [ ] SetSendCallback
  * [ ] SetErrorCallback
  * [X] GetBytesSent
  * [X] SendToAllClients
  * [X] SendToAllServers
  * [X] IsAlive
  * [X] GetRecvChan
* [X] IProxyContainer
  * [X] IsAlive
  * [X] SendToClient
  * [X] SendToServer
  * [X] GetId
  * [X] Network
  * [X] GetServerAddr
  * [X] GetClientAddr
  * [X] GetBytesSent
  * [X] LastContactAgo

I need to add the filtering callback now that its actually possible.

Also some sorta 'has filterer' would be cool so its not just catching errors, but hey who knows.