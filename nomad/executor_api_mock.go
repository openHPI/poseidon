// Code generated by mockery v2.8.0. DO NOT EDIT.

package nomad

import (
	context "context"

	api "github.com/hashicorp/nomad/api"

	io "io"

	mock "github.com/stretchr/testify/mock"

	url "net/url"
)

// ExecutorAPIMock is an autogenerated mock type for the ExecutorAPI type
type ExecutorAPIMock struct {
	mock.Mock
}

// AllocationStream provides a mock function with given fields: ctx
func (_m *ExecutorAPIMock) AllocationStream(ctx context.Context) (<-chan *api.Events, error) {
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
func (_m *ExecutorAPIMock) DeleteRunner(runnerId string) error {
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
func (_m *ExecutorAPIMock) EvaluationStream(evalID string, ctx context.Context) (<-chan *api.Events, error) {
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

// Execute provides a mock function with given fields: allocationID, ctx, command, tty, stdin, stdout, stderr
func (_m *ExecutorAPIMock) Execute(allocationID string, ctx context.Context, command []string, tty bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
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

// ExecuteCommand provides a mock function with given fields: jobID, ctx, command, tty, stdin, stdout, stderr
func (_m *ExecutorAPIMock) ExecuteCommand(jobID string, ctx context.Context, command []string, tty bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	ret := _m.Called(jobID, ctx, command, tty, stdin, stdout, stderr)

	var r0 int
	if rf, ok := ret.Get(0).(func(string, context.Context, []string, bool, io.Reader, io.Writer, io.Writer) int); ok {
		r0 = rf(jobID, ctx, command, tty, stdin, stdout, stderr)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, context.Context, []string, bool, io.Reader, io.Writer, io.Writer) error); ok {
		r1 = rf(jobID, ctx, command, tty, stdin, stdout, stderr)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// JobScale provides a mock function with given fields: jobId
func (_m *ExecutorAPIMock) JobScale(jobId string) (uint, error) {
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

// LoadAllJobs provides a mock function with given fields:
func (_m *ExecutorAPIMock) LoadAllJobs() ([]*api.Job, error) {
	ret := _m.Called()

	var r0 []*api.Job
	if rf, ok := ret.Get(0).(func() []*api.Job); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.Job)
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

// LoadJobList provides a mock function with given fields:
func (_m *ExecutorAPIMock) LoadJobList() ([]*api.JobListStub, error) {
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

// LoadRunners provides a mock function with given fields: jobID
func (_m *ExecutorAPIMock) LoadRunners(jobID string) ([]string, error) {
	ret := _m.Called(jobID)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(jobID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LoadTemplateJob provides a mock function with given fields: environmentID
func (_m *ExecutorAPIMock) LoadTemplateJob(environmentID string) (*api.Job, error) {
	ret := _m.Called(environmentID)

	var r0 *api.Job
	if rf, ok := ret.Get(0).(func(string) *api.Job); ok {
		r0 = rf(environmentID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.Job)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(environmentID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MarkRunnerAsUsed provides a mock function with given fields: runnerID
func (_m *ExecutorAPIMock) MarkRunnerAsUsed(runnerID string) error {
	ret := _m.Called(runnerID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(runnerID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MonitorEvaluation provides a mock function with given fields: evaluationID, ctx
func (_m *ExecutorAPIMock) MonitorEvaluation(evaluationID string, ctx context.Context) error {
	ret := _m.Called(evaluationID, ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, context.Context) error); ok {
		r0 = rf(evaluationID, ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RegisterNomadJob provides a mock function with given fields: job
func (_m *ExecutorAPIMock) RegisterNomadJob(job *api.Job) (string, error) {
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
func (_m *ExecutorAPIMock) SetJobScale(jobId string, count uint, reason string) error {
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
func (_m *ExecutorAPIMock) WatchAllocations(ctx context.Context, onNewAllocation AllocationProcessor, onDeletedAllocation AllocationProcessor) error {
	ret := _m.Called(ctx, onNewAllocation, onDeletedAllocation)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, AllocationProcessor, AllocationProcessor) error); ok {
		r0 = rf(ctx, onNewAllocation, onDeletedAllocation)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// init provides a mock function with given fields: nomadURL, nomadNamespace
func (_m *ExecutorAPIMock) init(nomadURL *url.URL, nomadNamespace string) error {
	ret := _m.Called(nomadURL, nomadNamespace)

	var r0 error
	if rf, ok := ret.Get(0).(func(*url.URL, string) error); ok {
		r0 = rf(nomadURL, nomadNamespace)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// jobInfo provides a mock function with given fields: jobID
func (_m *ExecutorAPIMock) jobInfo(jobID string) (*api.Job, error) {
	ret := _m.Called(jobID)

	var r0 *api.Job
	if rf, ok := ret.Get(0).(func(string) *api.Job); ok {
		r0 = rf(jobID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.Job)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// listJobs provides a mock function with given fields: prefix
func (_m *ExecutorAPIMock) listJobs(prefix string) ([]*api.JobListStub, error) {
	ret := _m.Called(prefix)

	var r0 []*api.JobListStub
	if rf, ok := ret.Get(0).(func(string) []*api.JobListStub); ok {
		r0 = rf(prefix)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.JobListStub)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(prefix)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
