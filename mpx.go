package main

import (
	"errors"
	"ezproxy/handler"
	"ezproxy/proxy"
)

// Returns the protocol of the listener ()
func GetMpxInfo(name string) (handler.PxProto, handler.IProxyListener, error) {
	switch name {
	case "TcpPlain":
		return handler.PxProtoTcp, proxy.TcpListener, nil
	case "UdpPlain":
		return handler.PxProtoUdp, proxy.UdpListener, nil
	case "UdpOverTcp":
		return handler.PxProtoTcp, proxy.UdpOverTcpListener, nil
	default:
		return 0, nil, errors.New("mpx not found")
	}
}
