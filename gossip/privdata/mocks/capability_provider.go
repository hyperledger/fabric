// Code generated by mockery v2.34.2. DO NOT EDIT.

package mocks

import (
	channelconfig "github.com/hyperledger/fabric/v3/common/channelconfig"
	mock "github.com/stretchr/testify/mock"
)

// CapabilityProvider is an autogenerated mock type for the CapabilityProvider type
type CapabilityProvider struct {
	mock.Mock
}

// Capabilities provides a mock function with given fields:
func (_m *CapabilityProvider) Capabilities() channelconfig.ApplicationCapabilities {
	ret := _m.Called()

	var r0 channelconfig.ApplicationCapabilities
	if rf, ok := ret.Get(0).(func() channelconfig.ApplicationCapabilities); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(channelconfig.ApplicationCapabilities)
		}
	}

	return r0
}

// NewCapabilityProvider creates a new instance of CapabilityProvider. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewCapabilityProvider(t interface {
	mock.TestingT
	Cleanup(func())
}) *CapabilityProvider {
	mock := &CapabilityProvider{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
