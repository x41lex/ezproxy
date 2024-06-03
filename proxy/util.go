package proxy

import "net"

// Check if this is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	nErr, ok := err.(net.Error)
	if !ok {
		return false
	}
	return nErr.Timeout()
}

func compareNetAddr(left net.Addr, right net.Addr) bool {
	return left.Network() == right.Network() && left.String() == right.String()
}
