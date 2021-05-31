// Code generated by mockery v2.8.0. DO NOT EDIT.

package nomad

import (
	context "context"

	api "github.com/hashicorp/nomad/api"

	io "io"

	mock "github.com/stretchr/testify/mock"

	url "net/url"
)

// ExecutorApiMock is an autogenerated mock type for the ExecutorApi type
type ExecutorApiMock struct {
	mock.Mock
}

// AllocationStream provides a mock function with given fields: ctx
func (_m *ExecutorApiMock) AllocationStream(ctx context.Context) (<-chan *api.Events, error) {
	ret := _m.Called(ctx)

	var r0 <-chan *api.Events
	if rf, ok := ret.Get(0).(func(context.Context) <-chan *api.Events); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan *api.Events)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteRunner provides a mock function with given fields: runnerId
func (_m *ExecutorApiMock) DeleteRunner(runnerId string) error {
	ret := _m.Called(runnerId)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(runnerId)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EvaluationStream provides a mock function with given fields: evalID, ctx
func (_m *ExecutorApiMock) EvaluationStream(evalID string, ctx context.Context) (<-chan *api.Events, error) {
	ret := _m.Called(evalID, ctx)

	var r0 <-chan *api.Events
	if rf, ok := ret.Get(0).(func(string, context.Context) <-chan *api.Events); ok {
		r0 = rf(evalID, ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan *api.Events)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, context.Context) error); ok {
		r1 = rf(evalID, ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ExecuteCommand provides a mock function with given fields: allocationID, ctx, command, tty, stdin, stdout, stderr
func (_m *ExecutorApiMock) ExecuteCommand(allocationID string, ctx context.Context, command []string, tty bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	ret := _m.Called(allocationID, ctx, command, tty, stdin, stdout, stderr)

	var r0 int
	if rf, ok := ret.Get(0).(func(string, context.Context, []string, bool, io.Reader, io.Writer, io.Writer) int); ok {
		r0 = rf(allocationID, ctx, command, tty, stdin, stdout, stderr)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, context.Context, []string, bool, io.Reader, io.Writer, io.Writer) error); ok {
		r1 = rf(allocationID, ctx, command, tty, stdin, stdout, stderr)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// JobScale provides a mock function with given fields: jobId
func (_m *ExecutorApiMock) JobScale(jobId string) (uint, error) {
	ret := _m.Called(jobId)

	var r0 uint
	if rf, ok := ret.Get(0).(func(string) uint); ok {
		r0 = rf(jobId)
	} else {
		r0 = ret.Get(0).(uint)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LoadJobList provides a mock function with given fields:
func (_m *ExecutorApiMock) LoadJobList() ([]*api.JobListStub, error) {
	ret := _m.Called()

	var r0 []*api.JobListStub
	if rf, ok := ret.Get(0).(func() []*api.JobListStub); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.JobListStub)
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

// LoadRunners provides a mock function with given fields: jobId
func (_m *ExecutorApiMock) LoadRunners(jobId string) ([]string, error) {
	ret := _m.Called(jobId)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(jobId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MonitorEvaluation provides a mock function with given fields: evalID, ctx
func (_m *ExecutorApiMock) MonitorEvaluation(evalID string, ctx context.Context) error {
	ret := _m.Called(evalID, ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, context.Context) error); ok {
		r0 = rf(evalID, ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RegisterNomadJob provides a mock function with given fields: job
func (_m *ExecutorApiMock) RegisterNomadJob(job *api.Job) (string, error) {
	ret := _m.Called(job)

	var r0 string
	if rf, ok := ret.Get(0).(func(*api.Job) string); ok {
		r0 = rf(job)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*api.Job) error); ok {
		r1 = rf(job)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SetJobScale provides a mock function with given fields: jobId, count, reason
func (_m *ExecutorApiMock) SetJobScale(jobId string, count uint, reason string) error {
	ret := _m.Called(jobId, count, reason)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, uint, string) error); ok {
		r0 = rf(jobId, count, reason)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// WatchAllocations provides a mock function with given fields: ctx, onNewAllocation, onDeletedAllocation
func (_m *ExecutorApiMock) WatchAllocations(ctx context.Context, onNewAllocation allocationProcessor, onDeletedAllocation allocationProcessor) error {
	ret := _m.Called(ctx, onNewAllocation, onDeletedAllocation)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, allocationProcessor, allocationProcessor) error); ok {
		r0 = rf(ctx, onNewAllocation, onDeletedAllocation)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// init provides a mock function with given fields: nomadURL, nomadNamespace
func (_m *ExecutorApiMock) init(nomadURL *url.URL, nomadNamespace string) error {
	ret := _m.Called(nomadURL, nomadNamespace)

	var r0 error
	if rf, ok := ret.Get(0).(func(*url.URL, string) error); ok {
		r0 = rf(nomadURL, nomadNamespace)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// loadRunners provides a mock function with given fields: jobId
func (_m *ExecutorApiMock) loadRunners(jobId string) ([]*api.AllocationListStub, error) {
	ret := _m.Called(jobId)

	var r0 []*api.AllocationListStub
	if rf, ok := ret.Get(0).(func(string) []*api.AllocationListStub); ok {
		r0 = rf(jobId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.AllocationListStub)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
