// Code generated by mockery v2.30.1. DO NOT EDIT.

package runner

import (
	context "context"
	http "net/http"

	dto "github.com/openHPI/poseidon/pkg/dto"

	io "io"

	mock "github.com/stretchr/testify/mock"

	time "time"
)

// RunnerMock is an autogenerated mock type for the Runner type
type RunnerMock struct {
	mock.Mock
}

// Destroy provides a mock function with given fields: local
func (_m *RunnerMock) Destroy(local bool) error {
	ret := _m.Called(local)

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(local)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Environment provides a mock function with given fields:
func (_m *RunnerMock) Environment() dto.EnvironmentID {
	ret := _m.Called()

	var r0 dto.EnvironmentID
	if rf, ok := ret.Get(0).(func() dto.EnvironmentID); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(dto.EnvironmentID)
	}

	return r0
}

// ExecuteInteractively provides a mock function with given fields: id, stdin, stdout, stderr, ctx
func (_m *RunnerMock) ExecuteInteractively(id string, stdin io.ReadWriter, stdout io.Writer, stderr io.Writer, ctx context.Context) (<-chan ExitInfo, context.CancelFunc, error) {
	ret := _m.Called(id, stdin, stdout, stderr, ctx)

	var r0 <-chan ExitInfo
	var r1 context.CancelFunc
	var r2 error
	if rf, ok := ret.Get(0).(func(string, io.ReadWriter, io.Writer, io.Writer, context.Context) (<-chan ExitInfo, context.CancelFunc, error)); ok {
		return rf(id, stdin, stdout, stderr, ctx)
	}
	if rf, ok := ret.Get(0).(func(string, io.ReadWriter, io.Writer, io.Writer, context.Context) <-chan ExitInfo); ok {
		r0 = rf(id, stdin, stdout, stderr, ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan ExitInfo)
		}
	}

	if rf, ok := ret.Get(1).(func(string, io.ReadWriter, io.Writer, io.Writer, context.Context) context.CancelFunc); ok {
		r1 = rf(id, stdin, stdout, stderr, ctx)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(context.CancelFunc)
		}
	}

	if rf, ok := ret.Get(2).(func(string, io.ReadWriter, io.Writer, io.Writer, context.Context) error); ok {
		r2 = rf(id, stdin, stdout, stderr, ctx)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// ExecutionExists provides a mock function with given fields: id
func (_m *RunnerMock) ExecutionExists(id string) bool {
	ret := _m.Called(id)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string) bool); ok {
		r0 = rf(id)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// GetFileContent provides a mock function with given fields: path, content, privilegedExecution, ctx
func (_m *RunnerMock) GetFileContent(path string, content http.ResponseWriter, privilegedExecution bool, ctx context.Context) error {
	ret := _m.Called(path, content, privilegedExecution, ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, http.ResponseWriter, bool, context.Context) error); ok {
		r0 = rf(path, content, privilegedExecution, ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ID provides a mock function with given fields:
func (_m *RunnerMock) ID() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// ListFileSystem provides a mock function with given fields: path, recursive, result, privilegedExecution, ctx
func (_m *RunnerMock) ListFileSystem(path string, recursive bool, result io.Writer, privilegedExecution bool, ctx context.Context) error {
	ret := _m.Called(path, recursive, result, privilegedExecution, ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, bool, io.Writer, bool, context.Context) error); ok {
		r0 = rf(path, recursive, result, privilegedExecution, ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MappedPorts provides a mock function with given fields:
func (_m *RunnerMock) MappedPorts() []*dto.MappedPort {
	ret := _m.Called()

	var r0 []*dto.MappedPort
	if rf, ok := ret.Get(0).(func() []*dto.MappedPort); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*dto.MappedPort)
		}
	}

	return r0
}

// ResetTimeout provides a mock function with given fields:
func (_m *RunnerMock) ResetTimeout() {
	_m.Called()
}

// SetupTimeout provides a mock function with given fields: duration
func (_m *RunnerMock) SetupTimeout(duration time.Duration) {
	_m.Called(duration)
}

// StopTimeout provides a mock function with given fields:
func (_m *RunnerMock) StopTimeout() {
	_m.Called()
}

// StoreExecution provides a mock function with given fields: id, executionRequest
func (_m *RunnerMock) StoreExecution(id string, executionRequest *dto.ExecutionRequest) {
	_m.Called(id, executionRequest)
}

// TimeoutPassed provides a mock function with given fields:
func (_m *RunnerMock) TimeoutPassed() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// UpdateFileSystem provides a mock function with given fields: request, ctx
func (_m *RunnerMock) UpdateFileSystem(request *dto.UpdateFileSystemRequest, ctx context.Context) error {
	ret := _m.Called(request, ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(*dto.UpdateFileSystemRequest, context.Context) error); ok {
		r0 = rf(request, ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewRunnerMock creates a new instance of RunnerMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewRunnerMock(t interface {
	mock.TestingT
	Cleanup(func())
}) *RunnerMock {
	mock := &RunnerMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
