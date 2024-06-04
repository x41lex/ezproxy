package handler

import "errors"

var (
	ErrSpawnerClosedOk error = errors.New("spawner closed")                     // Spawner was gracefully closed
	ErrProxyClosedOk   error = errors.New("proxy closed")                       // Proxy was gracefully closed
	ErrProxyMaxRetries error = errors.New("proxy retrying max number of times") // The proxy retried multiple times
	ErrProxyRetry      error = errors.New("proxy closed retrying")              // The proxy was closed but should be restarted, if this error is returned 3 times the proxy will be killed
)
