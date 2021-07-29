// Code generated by mockery v0.0.0-dev. DO NOT EDIT.

package nomad

import (
	context "context"

	api "github.com/hashicorp/nomad/api"

	io "io"

	mock "github.com/stretchr/testify/mock"

	url "net/url"
)

// apiQuerierMock is an autogenerated mock type for the apiQuerier type
type apiQuerierMock struct {
	mock.Mock
}

// AllocationStream provides a mock function with given fields: ctx
func (_m *apiQuerierMock) AllocationStream(ctx context.Context) (<-chan *api.Events, error) {
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

// DeleteRunner provides a mock function with given fields: runnerID
func (_m *apiQuerierMock) DeleteRunner(runnerID string) error {
	ret := _m.Called(runnerID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(runnerID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EvaluationStream provides a mock function with given fields: evalID, ctx
func (_m *apiQuerierMock) EvaluationStream(evalID string, ctx context.Context) (<-chan *api.Events, error) {
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

// Execute provides a mock function with given fields: jobID, ctx, command, tty, stdin, stdout, stderr
func (_m *apiQuerierMock) Execute(jobID string, ctx context.Context, command []string, tty bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
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

// JobScale provides a mock function with given fields: jobID
func (_m *apiQuerierMock) JobScale(jobID string) (uint, error) {
	ret := _m.Called(jobID)

	var r0 uint
	if rf, ok := ret.Get(0).(func(string) uint); ok {
		r0 = rf(jobID)
	} else {
		r0 = ret.Get(0).(uint)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LoadJobList provides a mock function with given fields:
func (_m *apiQuerierMock) LoadJobList() ([]*api.JobListStub, error) {
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

// RegisterNomadJob provides a mock function with given fields: job
func (_m *apiQuerierMock) RegisterNomadJob(job *api.Job) (string, error) {
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

// SetJobScale provides a mock function with given fields: jobID, count, reason
func (_m *apiQuerierMock) SetJobScale(jobID string, count uint, reason string) error {
	ret := _m.Called(jobID, count, reason)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, uint, string) error); ok {
		r0 = rf(jobID, count, reason)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// allocation provides a mock function with given fields: jobID
func (_m *apiQuerierMock) allocation(jobID string) (*api.Allocation, error) {
	ret := _m.Called(jobID)

	var r0 *api.Allocation
	if rf, ok := ret.Get(0).(func(string) *api.Allocation); ok {
		r0 = rf(jobID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.Allocation)
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

// init provides a mock function with given fields: nomadURL, nomadNamespace, nomadToken
func (_m *apiQuerierMock) init(nomadURL *url.URL, nomadNamespace string, nomadToken string) error {
	ret := _m.Called(nomadURL, nomadNamespace, nomadToken)

	var r0 error
	if rf, ok := ret.Get(0).(func(*url.URL, string, string) error); ok {
		r0 = rf(nomadURL, nomadNamespace, nomadToken)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// job provides a mock function with given fields: jobID
func (_m *apiQuerierMock) job(jobID string) (*api.Job, error) {
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
func (_m *apiQuerierMock) listJobs(prefix string) ([]*api.JobListStub, error) {
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