// Code generated by mockery v2.9.4. DO NOT EDIT.

package runner

import (
	dto "github.com/openHPI/poseidon/pkg/dto"
	mock "github.com/stretchr/testify/mock"

	nomad "github.com/openHPI/poseidon/internal/nomad"
)

// ExecutionEnvironmentMock is an autogenerated mock type for the ExecutionEnvironment type
type ExecutionEnvironmentMock struct {
	mock.Mock
}

// AddRunner provides a mock function with given fields: r
func (_m *ExecutionEnvironmentMock) AddRunner(r Runner) {
	_m.Called(r)
}

// ApplyPrewarmingPoolSize provides a mock function with given fields: apiClient
func (_m *ExecutionEnvironmentMock) ApplyPrewarmingPoolSize(apiClient nomad.ExecutorAPI) error {
	ret := _m.Called(apiClient)

	var r0 error
	if rf, ok := ret.Get(0).(func(nomad.ExecutorAPI) error); ok {
		r0 = rf(apiClient)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CPULimit provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) CPULimit() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

// Delete provides a mock function with given fields: apiClient
func (_m *ExecutionEnvironmentMock) Delete(apiClient nomad.ExecutorAPI) error {
	ret := _m.Called(apiClient)

	var r0 error
	if rf, ok := ret.Get(0).(func(nomad.ExecutorAPI) error); ok {
		r0 = rf(apiClient)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteRunner provides a mock function with given fields: id
func (_m *ExecutionEnvironmentMock) DeleteRunner(id string) {
	_m.Called(id)
}

// ID provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) ID() dto.EnvironmentID {
	ret := _m.Called()

	var r0 dto.EnvironmentID
	if rf, ok := ret.Get(0).(func() dto.EnvironmentID); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(dto.EnvironmentID)
	}

	return r0
}

// IdleRunnerCount provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) IdleRunnerCount() int {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// Image provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) Image() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MarshalJSON provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) MarshalJSON() ([]byte, error) {
	ret := _m.Called()

	var r0 []byte
	if rf, ok := ret.Get(0).(func() []byte); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MemoryLimit provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) MemoryLimit() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

// NetworkAccess provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) NetworkAccess() (bool, []uint16) {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 []uint16
	if rf, ok := ret.Get(1).(func() []uint16); ok {
		r1 = rf()
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]uint16)
		}
	}

	return r0, r1
}

// PrewarmingPoolSize provides a mock function with given fields:
func (_m *ExecutionEnvironmentMock) PrewarmingPoolSize() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

// Register provides a mock function with given fields: apiClient
func (_m *ExecutionEnvironmentMock) Register(apiClient nomad.ExecutorAPI) error {
	ret := _m.Called(apiClient)

	var r0 error
	if rf, ok := ret.Get(0).(func(nomad.ExecutorAPI) error); ok {
		r0 = rf(apiClient)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Sample provides a mock function with given fields: apiClient
func (_m *ExecutionEnvironmentMock) Sample(apiClient nomad.ExecutorAPI) (Runner, bool) {
	ret := _m.Called(apiClient)

	var r0 Runner
	if rf, ok := ret.Get(0).(func(nomad.ExecutorAPI) Runner); ok {
		r0 = rf(apiClient)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(Runner)
		}
	}

	var r1 bool
	if rf, ok := ret.Get(1).(func(nomad.ExecutorAPI) bool); ok {
		r1 = rf(apiClient)
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// SetCPULimit provides a mock function with given fields: limit
func (_m *ExecutionEnvironmentMock) SetCPULimit(limit uint) {
	_m.Called(limit)
}

// SetConfigFrom provides a mock function with given fields: environment
func (_m *ExecutionEnvironmentMock) SetConfigFrom(environment ExecutionEnvironment) {
	_m.Called(environment)
}

// SetID provides a mock function with given fields: id
func (_m *ExecutionEnvironmentMock) SetID(id dto.EnvironmentID) {
	_m.Called(id)
}

// SetImage provides a mock function with given fields: image
func (_m *ExecutionEnvironmentMock) SetImage(image string) {
	_m.Called(image)
}

// SetMemoryLimit provides a mock function with given fields: limit
func (_m *ExecutionEnvironmentMock) SetMemoryLimit(limit uint) {
	_m.Called(limit)
}

// SetNetworkAccess provides a mock function with given fields: allow, ports
func (_m *ExecutionEnvironmentMock) SetNetworkAccess(allow bool, ports []uint16) {
	_m.Called(allow, ports)
}

// SetPrewarmingPoolSize provides a mock function with given fields: count
func (_m *ExecutionEnvironmentMock) SetPrewarmingPoolSize(count uint) {
	_m.Called(count)
}
