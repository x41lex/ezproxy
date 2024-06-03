//go:build integration
// +build integration

package integration

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

func addrToTcpAddr(addr net.Addr) (*net.TCPAddr, error) {
	if tc, ok := addr.(*net.TCPAddr); ok {
		return tc, nil
	}
	return net.ResolveTCPAddr("tcp", addr.String())
}
