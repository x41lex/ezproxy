## TCP
## Connection DOS 
If the server fails to connect the TCP proxy fails but the proxy does not close, this likely happens with UDP too.

The proxy should either return `RestartProxy` or similar, `ProxyClosed` for the proxy closing gracefully, killing the spawner with it, or any other `error.Error` to return the error.
