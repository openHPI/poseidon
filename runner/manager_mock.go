// Code generated by mockery v2.8.0. DO NOT EDIT.

package runner

import mock "github.com/stretchr/testify/mock"

// ManagerMock is an autogenerated mock type for the Manager type
type ManagerMock struct {
	mock.Mock
}

// Claim provides a mock function with given fields: id
func (_m *ManagerMock) Claim(id EnvironmentID) (Runner, error) {
	ret := _m.Called(id)

	var r0 Runner
	if rf, ok := ret.Get(0).(func(EnvironmentID) Runner); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(Runner)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(EnvironmentID) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateOrUpdateEnvironment provides a mock function with given fields: id, desiredIdleRunnersCount
func (_m *ManagerMock) CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint) (bool, error) {
	ret := _m.Called(id, desiredIdleRunnersCount)

	var r0 bool
	if rf, ok := ret.Get(0).(func(EnvironmentID, uint) bool); ok {
		r0 = rf(id, desiredIdleRunnersCount)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(EnvironmentID, uint) error); ok {
		r1 = rf(id, desiredIdleRunnersCount)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Get provides a mock function with given fields: runnerID
func (_m *ManagerMock) Get(runnerID string) (Runner, error) {
	ret := _m.Called(runnerID)

	var r0 Runner
	if rf, ok := ret.Get(0).(func(string) Runner); ok {
		r0 = rf(runnerID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(Runner)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(runnerID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Return provides a mock function with given fields: r
func (_m *ManagerMock) Return(r Runner) error {
	ret := _m.Called(r)

	var r0 error
	if rf, ok := ret.Get(0).(func(Runner) error); ok {
		r0 = rf(r)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
