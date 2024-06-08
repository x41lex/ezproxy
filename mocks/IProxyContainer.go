// Code generated by mockery v2.42.2. DO NOT EDIT.

package mocks

import (
	net "net"

	mock "github.com/stretchr/testify/mock"

	time "time"
)

// IProxyContainer is an autogenerated mock type for the IProxyContainer type
type IProxyContainer struct {
	mock.Mock
}

// Cancel provides a mock function with given fields: cause
func (_m *IProxyContainer) Cancel(cause error) {
	_m.Called(cause)
}

// GetBytesSent provides a mock function with given fields:
func (_m *IProxyContainer) GetBytesSent() uint64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetBytesSent")
	}

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// GetClientAddr provides a mock function with given fields:
func (_m *IProxyContainer) GetClientAddr() net.Addr {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetClientAddr")
	}

	var r0 net.Addr
	if rf, ok := ret.Get(0).(func() net.Addr); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(net.Addr)
		}
	}

	return r0
}

// GetId provides a mock function with given fields:
func (_m *IProxyContainer) GetId() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetId")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// GetLastContactTime provides a mock function with given fields:
func (_m *IProxyContainer) GetLastContactTime() time.Time {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetLastContactTime")
	}

	var r0 time.Time
	if rf, ok := ret.Get(0).(func() time.Time); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Time)
	}

	return r0
}

// GetServerAddr provides a mock function with given fields:
func (_m *IProxyContainer) GetServerAddr() net.Addr {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetServerAddr")
	}

	var r0 net.Addr
	if rf, ok := ret.Get(0).(func() net.Addr); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(net.Addr)
		}
	}

	return r0
}

// IsAlive provides a mock function with given fields:
func (_m *IProxyContainer) IsAlive() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IsAlive")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// LastContactTimeAgo provides a mock function with given fields:
func (_m *IProxyContainer) LastContactTimeAgo() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LastContactTimeAgo")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// MpxName provides a mock function with given fields:
func (_m *IProxyContainer) MpxName() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for MpxName")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Network provides a mock function with given fields:
func (_m *IProxyContainer) Network() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Network")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// SendToClient provides a mock function with given fields: data
func (_m *IProxyContainer) SendToClient(data []byte) error {
	ret := _m.Called(data)

	if len(ret) == 0 {
		panic("no return value specified for SendToClient")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func([]byte) error); ok {
		r0 = rf(data)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SendToServer provides a mock function with given fields: data
func (_m *IProxyContainer) SendToServer(data []byte) error {
	ret := _m.Called(data)

	if len(ret) == 0 {
		panic("no return value specified for SendToServer")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func([]byte) error); ok {
		r0 = rf(data)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewIProxyContainer creates a new instance of IProxyContainer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewIProxyContainer(t interface {
	mock.TestingT
	Cleanup(func())
}) *IProxyContainer {
	mock := &IProxyContainer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
