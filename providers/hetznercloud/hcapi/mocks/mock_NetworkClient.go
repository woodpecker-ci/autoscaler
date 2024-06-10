// Code generated by mockery v2.42.1. DO NOT EDIT.

package mocks

import (
	context "context"

	hcloud "github.com/hetznercloud/hcloud-go/v2/hcloud"

	mock "github.com/stretchr/testify/mock"
)

// MockNetworkClient is an autogenerated mock type for the NetworkClient type
type MockNetworkClient struct {
	mock.Mock
}

type MockNetworkClient_Expecter struct {
	mock *mock.Mock
}

func (_m *MockNetworkClient) EXPECT() *MockNetworkClient_Expecter {
	return &MockNetworkClient_Expecter{mock: &_m.Mock}
}

// AddRoute provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) AddRoute(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkAddRouteOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for AddRoute")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkAddRouteOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkAddRouteOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkAddRouteOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkAddRouteOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_AddRoute_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddRoute'
type MockNetworkClient_AddRoute_Call struct {
	*mock.Call
}

// AddRoute is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkAddRouteOpts
func (_e *MockNetworkClient_Expecter) AddRoute(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_AddRoute_Call {
	return &MockNetworkClient_AddRoute_Call{Call: _e.mock.On("AddRoute", ctx, network, opts)}
}

func (_c *MockNetworkClient_AddRoute_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkAddRouteOpts)) *MockNetworkClient_AddRoute_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkAddRouteOpts))
	})
	return _c
}

func (_c *MockNetworkClient_AddRoute_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_AddRoute_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_AddRoute_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkAddRouteOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_AddRoute_Call {
	_c.Call.Return(run)
	return _c
}

// AddSubnet provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) AddSubnet(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkAddSubnetOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for AddSubnet")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkAddSubnetOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkAddSubnetOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkAddSubnetOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkAddSubnetOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_AddSubnet_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddSubnet'
type MockNetworkClient_AddSubnet_Call struct {
	*mock.Call
}

// AddSubnet is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkAddSubnetOpts
func (_e *MockNetworkClient_Expecter) AddSubnet(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_AddSubnet_Call {
	return &MockNetworkClient_AddSubnet_Call{Call: _e.mock.On("AddSubnet", ctx, network, opts)}
}

func (_c *MockNetworkClient_AddSubnet_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkAddSubnetOpts)) *MockNetworkClient_AddSubnet_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkAddSubnetOpts))
	})
	return _c
}

func (_c *MockNetworkClient_AddSubnet_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_AddSubnet_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_AddSubnet_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkAddSubnetOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_AddSubnet_Call {
	_c.Call.Return(run)
	return _c
}

// All provides a mock function with given fields: ctx
func (_m *MockNetworkClient) All(ctx context.Context) ([]*hcloud.Network, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for All")
	}

	var r0 []*hcloud.Network
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) ([]*hcloud.Network, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) []*hcloud.Network); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockNetworkClient_All_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'All'
type MockNetworkClient_All_Call struct {
	*mock.Call
}

// All is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockNetworkClient_Expecter) All(ctx interface{}) *MockNetworkClient_All_Call {
	return &MockNetworkClient_All_Call{Call: _e.mock.On("All", ctx)}
}

func (_c *MockNetworkClient_All_Call) Run(run func(ctx context.Context)) *MockNetworkClient_All_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockNetworkClient_All_Call) Return(_a0 []*hcloud.Network, _a1 error) *MockNetworkClient_All_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockNetworkClient_All_Call) RunAndReturn(run func(context.Context) ([]*hcloud.Network, error)) *MockNetworkClient_All_Call {
	_c.Call.Return(run)
	return _c
}

// AllWithOpts provides a mock function with given fields: ctx, opts
func (_m *MockNetworkClient) AllWithOpts(ctx context.Context, opts hcloud.NetworkListOpts) ([]*hcloud.Network, error) {
	ret := _m.Called(ctx, opts)

	if len(ret) == 0 {
		panic("no return value specified for AllWithOpts")
	}

	var r0 []*hcloud.Network
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkListOpts) ([]*hcloud.Network, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkListOpts) []*hcloud.Network); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, hcloud.NetworkListOpts) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockNetworkClient_AllWithOpts_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AllWithOpts'
type MockNetworkClient_AllWithOpts_Call struct {
	*mock.Call
}

// AllWithOpts is a helper method to define mock.On call
//   - ctx context.Context
//   - opts hcloud.NetworkListOpts
func (_e *MockNetworkClient_Expecter) AllWithOpts(ctx interface{}, opts interface{}) *MockNetworkClient_AllWithOpts_Call {
	return &MockNetworkClient_AllWithOpts_Call{Call: _e.mock.On("AllWithOpts", ctx, opts)}
}

func (_c *MockNetworkClient_AllWithOpts_Call) Run(run func(ctx context.Context, opts hcloud.NetworkListOpts)) *MockNetworkClient_AllWithOpts_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(hcloud.NetworkListOpts))
	})
	return _c
}

func (_c *MockNetworkClient_AllWithOpts_Call) Return(_a0 []*hcloud.Network, _a1 error) *MockNetworkClient_AllWithOpts_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockNetworkClient_AllWithOpts_Call) RunAndReturn(run func(context.Context, hcloud.NetworkListOpts) ([]*hcloud.Network, error)) *MockNetworkClient_AllWithOpts_Call {
	_c.Call.Return(run)
	return _c
}

// ChangeIPRange provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) ChangeIPRange(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkChangeIPRangeOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for ChangeIPRange")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeIPRangeOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeIPRangeOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeIPRangeOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeIPRangeOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_ChangeIPRange_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ChangeIPRange'
type MockNetworkClient_ChangeIPRange_Call struct {
	*mock.Call
}

// ChangeIPRange is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkChangeIPRangeOpts
func (_e *MockNetworkClient_Expecter) ChangeIPRange(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_ChangeIPRange_Call {
	return &MockNetworkClient_ChangeIPRange_Call{Call: _e.mock.On("ChangeIPRange", ctx, network, opts)}
}

func (_c *MockNetworkClient_ChangeIPRange_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkChangeIPRangeOpts)) *MockNetworkClient_ChangeIPRange_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkChangeIPRangeOpts))
	})
	return _c
}

func (_c *MockNetworkClient_ChangeIPRange_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_ChangeIPRange_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_ChangeIPRange_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkChangeIPRangeOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_ChangeIPRange_Call {
	_c.Call.Return(run)
	return _c
}

// ChangeProtection provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) ChangeProtection(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkChangeProtectionOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for ChangeProtection")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeProtectionOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeProtectionOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeProtectionOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkChangeProtectionOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_ChangeProtection_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ChangeProtection'
type MockNetworkClient_ChangeProtection_Call struct {
	*mock.Call
}

// ChangeProtection is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkChangeProtectionOpts
func (_e *MockNetworkClient_Expecter) ChangeProtection(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_ChangeProtection_Call {
	return &MockNetworkClient_ChangeProtection_Call{Call: _e.mock.On("ChangeProtection", ctx, network, opts)}
}

func (_c *MockNetworkClient_ChangeProtection_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkChangeProtectionOpts)) *MockNetworkClient_ChangeProtection_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkChangeProtectionOpts))
	})
	return _c
}

func (_c *MockNetworkClient_ChangeProtection_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_ChangeProtection_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_ChangeProtection_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkChangeProtectionOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_ChangeProtection_Call {
	_c.Call.Return(run)
	return _c
}

// Create provides a mock function with given fields: ctx, opts
func (_m *MockNetworkClient) Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, opts)

	if len(ret) == 0 {
		panic("no return value specified for Create")
	}

	var r0 *hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkCreateOpts) *hcloud.Network); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, hcloud.NetworkCreateOpts) *hcloud.Response); ok {
		r1 = rf(ctx, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, hcloud.NetworkCreateOpts) error); ok {
		r2 = rf(ctx, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_Create_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Create'
type MockNetworkClient_Create_Call struct {
	*mock.Call
}

// Create is a helper method to define mock.On call
//   - ctx context.Context
//   - opts hcloud.NetworkCreateOpts
func (_e *MockNetworkClient_Expecter) Create(ctx interface{}, opts interface{}) *MockNetworkClient_Create_Call {
	return &MockNetworkClient_Create_Call{Call: _e.mock.On("Create", ctx, opts)}
}

func (_c *MockNetworkClient_Create_Call) Run(run func(ctx context.Context, opts hcloud.NetworkCreateOpts)) *MockNetworkClient_Create_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(hcloud.NetworkCreateOpts))
	})
	return _c
}

func (_c *MockNetworkClient_Create_Call) Return(_a0 *hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_Create_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_Create_Call) RunAndReturn(run func(context.Context, hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_Create_Call {
	_c.Call.Return(run)
	return _c
}

// Delete provides a mock function with given fields: ctx, network
func (_m *MockNetworkClient) Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error) {
	ret := _m.Called(ctx, network)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 *hcloud.Response
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network) (*hcloud.Response, error)); ok {
		return rf(ctx, network)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network) *hcloud.Response); ok {
		r0 = rf(ctx, network)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network) error); ok {
		r1 = rf(ctx, network)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockNetworkClient_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type MockNetworkClient_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
func (_e *MockNetworkClient_Expecter) Delete(ctx interface{}, network interface{}) *MockNetworkClient_Delete_Call {
	return &MockNetworkClient_Delete_Call{Call: _e.mock.On("Delete", ctx, network)}
}

func (_c *MockNetworkClient_Delete_Call) Run(run func(ctx context.Context, network *hcloud.Network)) *MockNetworkClient_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network))
	})
	return _c
}

func (_c *MockNetworkClient_Delete_Call) Return(_a0 *hcloud.Response, _a1 error) *MockNetworkClient_Delete_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockNetworkClient_Delete_Call) RunAndReturn(run func(context.Context, *hcloud.Network) (*hcloud.Response, error)) *MockNetworkClient_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteRoute provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) DeleteRoute(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkDeleteRouteOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for DeleteRoute")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteRouteOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteRouteOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteRouteOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteRouteOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_DeleteRoute_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteRoute'
type MockNetworkClient_DeleteRoute_Call struct {
	*mock.Call
}

// DeleteRoute is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkDeleteRouteOpts
func (_e *MockNetworkClient_Expecter) DeleteRoute(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_DeleteRoute_Call {
	return &MockNetworkClient_DeleteRoute_Call{Call: _e.mock.On("DeleteRoute", ctx, network, opts)}
}

func (_c *MockNetworkClient_DeleteRoute_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkDeleteRouteOpts)) *MockNetworkClient_DeleteRoute_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkDeleteRouteOpts))
	})
	return _c
}

func (_c *MockNetworkClient_DeleteRoute_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_DeleteRoute_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_DeleteRoute_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkDeleteRouteOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_DeleteRoute_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteSubnet provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) DeleteSubnet(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkDeleteSubnetOpts) (*hcloud.Action, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for DeleteSubnet")
	}

	var r0 *hcloud.Action
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteSubnetOpts) (*hcloud.Action, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteSubnetOpts) *hcloud.Action); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Action)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteSubnetOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkDeleteSubnetOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_DeleteSubnet_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteSubnet'
type MockNetworkClient_DeleteSubnet_Call struct {
	*mock.Call
}

// DeleteSubnet is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkDeleteSubnetOpts
func (_e *MockNetworkClient_Expecter) DeleteSubnet(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_DeleteSubnet_Call {
	return &MockNetworkClient_DeleteSubnet_Call{Call: _e.mock.On("DeleteSubnet", ctx, network, opts)}
}

func (_c *MockNetworkClient_DeleteSubnet_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkDeleteSubnetOpts)) *MockNetworkClient_DeleteSubnet_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkDeleteSubnetOpts))
	})
	return _c
}

func (_c *MockNetworkClient_DeleteSubnet_Call) Return(_a0 *hcloud.Action, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_DeleteSubnet_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_DeleteSubnet_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkDeleteSubnetOpts) (*hcloud.Action, *hcloud.Response, error)) *MockNetworkClient_DeleteSubnet_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields: ctx, idOrName
func (_m *MockNetworkClient) Get(ctx context.Context, idOrName string) (*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, idOrName)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 *hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, idOrName)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *hcloud.Network); ok {
		r0 = rf(ctx, idOrName)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) *hcloud.Response); ok {
		r1 = rf(ctx, idOrName)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string) error); ok {
		r2 = rf(ctx, idOrName)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockNetworkClient_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - idOrName string
func (_e *MockNetworkClient_Expecter) Get(ctx interface{}, idOrName interface{}) *MockNetworkClient_Get_Call {
	return &MockNetworkClient_Get_Call{Call: _e.mock.On("Get", ctx, idOrName)}
}

func (_c *MockNetworkClient_Get_Call) Run(run func(ctx context.Context, idOrName string)) *MockNetworkClient_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockNetworkClient_Get_Call) Return(_a0 *hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_Get_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_Get_Call) RunAndReturn(run func(context.Context, string) (*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_Get_Call {
	_c.Call.Return(run)
	return _c
}

// GetByID provides a mock function with given fields: ctx, id
func (_m *MockNetworkClient) GetByID(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for GetByID")
	}

	var r0 *hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, int64) (*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, int64) *hcloud.Network); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, int64) *hcloud.Response); ok {
		r1 = rf(ctx, id)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, int64) error); ok {
		r2 = rf(ctx, id)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_GetByID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetByID'
type MockNetworkClient_GetByID_Call struct {
	*mock.Call
}

// GetByID is a helper method to define mock.On call
//   - ctx context.Context
//   - id int64
func (_e *MockNetworkClient_Expecter) GetByID(ctx interface{}, id interface{}) *MockNetworkClient_GetByID_Call {
	return &MockNetworkClient_GetByID_Call{Call: _e.mock.On("GetByID", ctx, id)}
}

func (_c *MockNetworkClient_GetByID_Call) Run(run func(ctx context.Context, id int64)) *MockNetworkClient_GetByID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(int64))
	})
	return _c
}

func (_c *MockNetworkClient_GetByID_Call) Return(_a0 *hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_GetByID_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_GetByID_Call) RunAndReturn(run func(context.Context, int64) (*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_GetByID_Call {
	_c.Call.Return(run)
	return _c
}

// GetByName provides a mock function with given fields: ctx, name
func (_m *MockNetworkClient) GetByName(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for GetByName")
	}

	var r0 *hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *hcloud.Network); ok {
		r0 = rf(ctx, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) *hcloud.Response); ok {
		r1 = rf(ctx, name)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string) error); ok {
		r2 = rf(ctx, name)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_GetByName_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetByName'
type MockNetworkClient_GetByName_Call struct {
	*mock.Call
}

// GetByName is a helper method to define mock.On call
//   - ctx context.Context
//   - name string
func (_e *MockNetworkClient_Expecter) GetByName(ctx interface{}, name interface{}) *MockNetworkClient_GetByName_Call {
	return &MockNetworkClient_GetByName_Call{Call: _e.mock.On("GetByName", ctx, name)}
}

func (_c *MockNetworkClient_GetByName_Call) Run(run func(ctx context.Context, name string)) *MockNetworkClient_GetByName_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockNetworkClient_GetByName_Call) Return(_a0 *hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_GetByName_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_GetByName_Call) RunAndReturn(run func(context.Context, string) (*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_GetByName_Call {
	_c.Call.Return(run)
	return _c
}

// List provides a mock function with given fields: ctx, opts
func (_m *MockNetworkClient) List(ctx context.Context, opts hcloud.NetworkListOpts) ([]*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, opts)

	if len(ret) == 0 {
		panic("no return value specified for List")
	}

	var r0 []*hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkListOpts) ([]*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, hcloud.NetworkListOpts) []*hcloud.Network); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, hcloud.NetworkListOpts) *hcloud.Response); ok {
		r1 = rf(ctx, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, hcloud.NetworkListOpts) error); ok {
		r2 = rf(ctx, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_List_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'List'
type MockNetworkClient_List_Call struct {
	*mock.Call
}

// List is a helper method to define mock.On call
//   - ctx context.Context
//   - opts hcloud.NetworkListOpts
func (_e *MockNetworkClient_Expecter) List(ctx interface{}, opts interface{}) *MockNetworkClient_List_Call {
	return &MockNetworkClient_List_Call{Call: _e.mock.On("List", ctx, opts)}
}

func (_c *MockNetworkClient_List_Call) Run(run func(ctx context.Context, opts hcloud.NetworkListOpts)) *MockNetworkClient_List_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(hcloud.NetworkListOpts))
	})
	return _c
}

func (_c *MockNetworkClient_List_Call) Return(_a0 []*hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_List_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_List_Call) RunAndReturn(run func(context.Context, hcloud.NetworkListOpts) ([]*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_List_Call {
	_c.Call.Return(run)
	return _c
}

// Update provides a mock function with given fields: ctx, network, opts
func (_m *MockNetworkClient) Update(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkUpdateOpts) (*hcloud.Network, *hcloud.Response, error) {
	ret := _m.Called(ctx, network, opts)

	if len(ret) == 0 {
		panic("no return value specified for Update")
	}

	var r0 *hcloud.Network
	var r1 *hcloud.Response
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkUpdateOpts) (*hcloud.Network, *hcloud.Response, error)); ok {
		return rf(ctx, network, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *hcloud.Network, hcloud.NetworkUpdateOpts) *hcloud.Network); ok {
		r0 = rf(ctx, network, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hcloud.Network)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *hcloud.Network, hcloud.NetworkUpdateOpts) *hcloud.Response); ok {
		r1 = rf(ctx, network, opts)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*hcloud.Response)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *hcloud.Network, hcloud.NetworkUpdateOpts) error); ok {
		r2 = rf(ctx, network, opts)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockNetworkClient_Update_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Update'
type MockNetworkClient_Update_Call struct {
	*mock.Call
}

// Update is a helper method to define mock.On call
//   - ctx context.Context
//   - network *hcloud.Network
//   - opts hcloud.NetworkUpdateOpts
func (_e *MockNetworkClient_Expecter) Update(ctx interface{}, network interface{}, opts interface{}) *MockNetworkClient_Update_Call {
	return &MockNetworkClient_Update_Call{Call: _e.mock.On("Update", ctx, network, opts)}
}

func (_c *MockNetworkClient_Update_Call) Run(run func(ctx context.Context, network *hcloud.Network, opts hcloud.NetworkUpdateOpts)) *MockNetworkClient_Update_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*hcloud.Network), args[2].(hcloud.NetworkUpdateOpts))
	})
	return _c
}

func (_c *MockNetworkClient_Update_Call) Return(_a0 *hcloud.Network, _a1 *hcloud.Response, _a2 error) *MockNetworkClient_Update_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *MockNetworkClient_Update_Call) RunAndReturn(run func(context.Context, *hcloud.Network, hcloud.NetworkUpdateOpts) (*hcloud.Network, *hcloud.Response, error)) *MockNetworkClient_Update_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockNetworkClient creates a new instance of MockNetworkClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockNetworkClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockNetworkClient {
	mock := &MockNetworkClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
