// Code generated by mockery v0.0.0-dev. DO NOT EDIT.

package runner

import (
	api "github.com/hashicorp/nomad/api"
	mock "github.com/stretchr/testify/mock"
)

// ManagerMock is an autogenerated mock type for the Manager type
type ManagerMock struct {
	mock.Mock
}

// Claim provides a mock function with given fields: id, duration
func (_m *ManagerMock) Claim(id EnvironmentID, duration int) (Runner, error) {
	ret := _m.Called(id, duration)

	var r0 Runner
	if rf, ok := ret.Get(0).(func(EnvironmentID, int) Runner); ok {
		r0 = rf(id, duration)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(Runner)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(EnvironmentID, int) error); ok {
		r1 = rf(id, duration)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateOrUpdateEnvironment provides a mock function with given fields: id, desiredIdleRunnersCount, templateJob, scale
func (_m *ManagerMock) CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint, templateJob *api.Job, scale bool) (bool, error) {
	ret := _m.Called(id, desiredIdleRunnersCount, templateJob, scale)

	var r0 bool
	if rf, ok := ret.Get(0).(func(EnvironmentID, uint, *api.Job, bool) bool); ok {
		r0 = rf(id, desiredIdleRunnersCount, templateJob, scale)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(EnvironmentID, uint, *api.Job, bool) error); ok {
		r1 = rf(id, desiredIdleRunnersCount, templateJob, scale)
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

// Load provides a mock function with given fields:
func (_m *ManagerMock) Load() {
	_m.Called()
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

// ScaleAllEnvironments provides a mock function with given fields:
func (_m *ManagerMock) ScaleAllEnvironments() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
