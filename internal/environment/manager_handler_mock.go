// Code generated by mockery v2.16.0. DO NOT EDIT.

package environment

import (
	context "context"

	dto "github.com/openHPI/poseidon/pkg/dto"
	mock "github.com/stretchr/testify/mock"

	runner "github.com/openHPI/poseidon/internal/runner"
)

// ManagerHandlerMock is an autogenerated mock type for the ManagerHandler type
type ManagerHandlerMock struct {
	mock.Mock
}

// CreateOrUpdate provides a mock function with given fields: id, request, ctx
func (_m *ManagerHandlerMock) CreateOrUpdate(id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest, ctx context.Context) (bool, error) {
	ret := _m.Called(id, request, ctx)

	var r0 bool
	if rf, ok := ret.Get(0).(func(dto.EnvironmentID, dto.ExecutionEnvironmentRequest, context.Context) bool); ok {
		r0 = rf(id, request, ctx)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(dto.EnvironmentID, dto.ExecutionEnvironmentRequest, context.Context) error); ok {
		r1 = rf(id, request, ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: id
func (_m *ManagerHandlerMock) Delete(id dto.EnvironmentID) (bool, error) {
	ret := _m.Called(id)

	var r0 bool
	if rf, ok := ret.Get(0).(func(dto.EnvironmentID) bool); ok {
		r0 = rf(id)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(dto.EnvironmentID) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Get provides a mock function with given fields: id, fetch
func (_m *ManagerHandlerMock) Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error) {
	ret := _m.Called(id, fetch)

	var r0 runner.ExecutionEnvironment
	if rf, ok := ret.Get(0).(func(dto.EnvironmentID, bool) runner.ExecutionEnvironment); ok {
		r0 = rf(id, fetch)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runner.ExecutionEnvironment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(dto.EnvironmentID, bool) error); ok {
		r1 = rf(id, fetch)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// HasNextHandler provides a mock function with given fields:
func (_m *ManagerHandlerMock) HasNextHandler() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// List provides a mock function with given fields: fetch
func (_m *ManagerHandlerMock) List(fetch bool) ([]runner.ExecutionEnvironment, error) {
	ret := _m.Called(fetch)

	var r0 []runner.ExecutionEnvironment
	if rf, ok := ret.Get(0).(func(bool) []runner.ExecutionEnvironment); ok {
		r0 = rf(fetch)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]runner.ExecutionEnvironment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(bool) error); ok {
		r1 = rf(fetch)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NextHandler provides a mock function with given fields:
func (_m *ManagerHandlerMock) NextHandler() ManagerHandler {
	ret := _m.Called()

	var r0 ManagerHandler
	if rf, ok := ret.Get(0).(func() ManagerHandler); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(ManagerHandler)
		}
	}

	return r0
}

// SetNextHandler provides a mock function with given fields: next
func (_m *ManagerHandlerMock) SetNextHandler(next ManagerHandler) {
	_m.Called(next)
}

// Statistics provides a mock function with given fields:
func (_m *ManagerHandlerMock) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	ret := _m.Called()

	var r0 map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData
	if rf, ok := ret.Get(0).(func() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData)
		}
	}

	return r0
}

type mockConstructorTestingTNewManagerHandlerMock interface {
	mock.TestingT
	Cleanup(func())
}

// NewManagerHandlerMock creates a new instance of ManagerHandlerMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewManagerHandlerMock(t mockConstructorTestingTNewManagerHandlerMock) *ManagerHandlerMock {
	mock := &ManagerHandlerMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
