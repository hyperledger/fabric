// Code generated by mockery v2.30.16. DO NOT EDIT.

package mocks

import (
	policies "github.com/hyperledger/fabric/common/policies"
	mock "github.com/stretchr/testify/mock"
)

// PolicyManager is an autogenerated mock type for the PolicyManager type
type PolicyManager struct {
	mock.Mock
}

type PolicyManager_Expecter struct {
	mock *mock.Mock
}

func (_m *PolicyManager) EXPECT() *PolicyManager_Expecter {
	return &PolicyManager_Expecter{mock: &_m.Mock}
}

// GetPolicy provides a mock function with given fields: id
func (_m *PolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	ret := _m.Called(id)

	var r0 policies.Policy
	var r1 bool
	if rf, ok := ret.Get(0).(func(string) (policies.Policy, bool)); ok {
		return rf(id)
	}
	if rf, ok := ret.Get(0).(func(string) policies.Policy); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(policies.Policy)
		}
	}

	if rf, ok := ret.Get(1).(func(string) bool); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// PolicyManager_GetPolicy_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPolicy'
type PolicyManager_GetPolicy_Call struct {
	*mock.Call
}

// GetPolicy is a helper method to define mock.On call
//   - id string
func (_e *PolicyManager_Expecter) GetPolicy(id interface{}) *PolicyManager_GetPolicy_Call {
	return &PolicyManager_GetPolicy_Call{Call: _e.mock.On("GetPolicy", id)}
}

func (_c *PolicyManager_GetPolicy_Call) Run(run func(id string)) *PolicyManager_GetPolicy_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string))
	})
	return _c
}

func (_c *PolicyManager_GetPolicy_Call) Return(_a0 policies.Policy, _a1 bool) *PolicyManager_GetPolicy_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *PolicyManager_GetPolicy_Call) RunAndReturn(run func(string) (policies.Policy, bool)) *PolicyManager_GetPolicy_Call {
	_c.Call.Return(run)
	return _c
}

// Manager provides a mock function with given fields: path
func (_m *PolicyManager) Manager(path []string) (policies.Manager, bool) {
	ret := _m.Called(path)

	var r0 policies.Manager
	var r1 bool
	if rf, ok := ret.Get(0).(func([]string) (policies.Manager, bool)); ok {
		return rf(path)
	}
	if rf, ok := ret.Get(0).(func([]string) policies.Manager); ok {
		r0 = rf(path)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(policies.Manager)
		}
	}

	if rf, ok := ret.Get(1).(func([]string) bool); ok {
		r1 = rf(path)
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// PolicyManager_Manager_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Manager'
type PolicyManager_Manager_Call struct {
	*mock.Call
}

// Manager is a helper method to define mock.On call
//   - path []string
func (_e *PolicyManager_Expecter) Manager(path interface{}) *PolicyManager_Manager_Call {
	return &PolicyManager_Manager_Call{Call: _e.mock.On("Manager", path)}
}

func (_c *PolicyManager_Manager_Call) Run(run func(path []string)) *PolicyManager_Manager_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].([]string))
	})
	return _c
}

func (_c *PolicyManager_Manager_Call) Return(_a0 policies.Manager, _a1 bool) *PolicyManager_Manager_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *PolicyManager_Manager_Call) RunAndReturn(run func([]string) (policies.Manager, bool)) *PolicyManager_Manager_Call {
	_c.Call.Return(run)
	return _c
}

// NewPolicyManager creates a new instance of PolicyManager. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewPolicyManager(t interface {
	mock.TestingT
	Cleanup(func())
}) *PolicyManager {
	mock := &PolicyManager{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
