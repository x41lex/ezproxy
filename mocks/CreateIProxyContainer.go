// Code generated by mockery v2.42.2. DO NOT EDIT.

package mocks

import (
	handler "ezproxy/handler"

	mock "github.com/stretchr/testify/mock"
)

// CreateIProxyContainer is an autogenerated mock type for the CreateIProxyContainer type
type CreateIProxyContainer struct {
	mock.Mock
}

// Execute provides a mock function with given fields: parent, px, id
func (_m *CreateIProxyContainer) Execute(parent handler.IProxySpawner, px handler.IProxy, id int) (handler.IProxyContainer, error) {
	ret := _m.Called(parent, px, id)

	if len(ret) == 0 {
		panic("no return value specified for Execute")
	}

	var r0 handler.IProxyContainer
	var r1 error
	if rf, ok := ret.Get(0).(func(handler.IProxySpawner, handler.IProxy, int) (handler.IProxyContainer, error)); ok {
		return rf(parent, px, id)
	}
	if rf, ok := ret.Get(0).(func(handler.IProxySpawner, handler.IProxy, int) handler.IProxyContainer); ok {
		r0 = rf(parent, px, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(handler.IProxyContainer)
		}
	}

	if rf, ok := ret.Get(1).(func(handler.IProxySpawner, handler.IProxy, int) error); ok {
		r1 = rf(parent, px, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewCreateIProxyContainer creates a new instance of CreateIProxyContainer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewCreateIProxyContainer(t interface {
	mock.TestingT
	Cleanup(func())
}) *CreateIProxyContainer {
	mock := &CreateIProxyContainer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
