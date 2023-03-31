// Code generated by mockery v2.23.1. DO NOT EDIT.

package nomad

import (
	context "context"

	api "github.com/hashicorp/nomad/api"
	config "github.com/openHPI/poseidon/internal/config"

	io "io"

	mock "github.com/stretchr/testify/mock"
)

// apiQuerierMock is an autogenerated mock type for the apiQuerier type
type apiQuerierMock struct {
	mock.Mock
}

// DeleteJob provides a mock function with given fields: jobID
func (_m *apiQuerierMock) DeleteJob(jobID string) error {
	ret := _m.Called(jobID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(jobID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// EventStream provides a mock function with given fields: ctx
func (_m *apiQuerierMock) EventStream(ctx context.Context) (<-chan *api.Events, error) {
	ret := _m.Called(ctx)

	var r0 <-chan *api.Events
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (<-chan *api.Events, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) <-chan *api.Events); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan *api.Events)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Execute provides a mock function with given fields: jobID, ctx, command, tty, stdin, stdout, stderr
func (_m *apiQuerierMock) Execute(jobID string, ctx context.Context, command string, tty bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	ret := _m.Called(jobID, ctx, command, tty, stdin, stdout, stderr)

	var r0 int
	var r1 error
	if rf, ok := ret.Get(0).(func(string, context.Context, string, bool, io.Reader, io.Writer, io.Writer) (int, error)); ok {
		return rf(jobID, ctx, command, tty, stdin, stdout, stderr)
	}
	if rf, ok := ret.Get(0).(func(string, context.Context, string, bool, io.Reader, io.Writer, io.Writer) int); ok {
		r0 = rf(jobID, ctx, command, tty, stdin, stdout, stderr)
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func(string, context.Context, string, bool, io.Reader, io.Writer, io.Writer) error); ok {
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
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (uint, error)); ok {
		return rf(jobID)
	}
	if rf, ok := ret.Get(0).(func(string) uint); ok {
		r0 = rf(jobID)
	} else {
		r0 = ret.Get(0).(uint)
	}

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
	var r1 error
	if rf, ok := ret.Get(0).(func() ([]*api.JobListStub, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() []*api.JobListStub); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.JobListStub)
		}
	}

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
	var r1 error
	if rf, ok := ret.Get(0).(func(*api.Job) (string, error)); ok {
		return rf(job)
	}
	if rf, ok := ret.Get(0).(func(*api.Job) string); ok {
		r0 = rf(job)
	} else {
		r0 = ret.Get(0).(string)
	}

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
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (*api.Allocation, error)); ok {
		return rf(jobID)
	}
	if rf, ok := ret.Get(0).(func(string) *api.Allocation); ok {
		r0 = rf(jobID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.Allocation)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// init provides a mock function with given fields: nomadConfig
func (_m *apiQuerierMock) init(nomadConfig *config.Nomad) error {
	ret := _m.Called(nomadConfig)

	var r0 error
	if rf, ok := ret.Get(0).(func(*config.Nomad) error); ok {
		r0 = rf(nomadConfig)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// job provides a mock function with given fields: jobID
func (_m *apiQuerierMock) job(jobID string) (*api.Job, error) {
	ret := _m.Called(jobID)

	var r0 *api.Job
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (*api.Job, error)); ok {
		return rf(jobID)
	}
	if rf, ok := ret.Get(0).(func(string) *api.Job); ok {
		r0 = rf(jobID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.Job)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(jobID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// listAllocations provides a mock function with given fields:
func (_m *apiQuerierMock) listAllocations() ([]*api.AllocationListStub, error) {
	ret := _m.Called()

	var r0 []*api.AllocationListStub
	var r1 error
	if rf, ok := ret.Get(0).(func() ([]*api.AllocationListStub, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() []*api.AllocationListStub); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.AllocationListStub)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// listJobs provides a mock function with given fields: prefix
func (_m *apiQuerierMock) listJobs(prefix string) ([]*api.JobListStub, error) {
	ret := _m.Called(prefix)

	var r0 []*api.JobListStub
	var r1 error
	if rf, ok := ret.Get(0).(func(string) ([]*api.JobListStub, error)); ok {
		return rf(prefix)
	}
	if rf, ok := ret.Get(0).(func(string) []*api.JobListStub); ok {
		r0 = rf(prefix)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*api.JobListStub)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(prefix)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTnewApiQuerierMock interface {
	mock.TestingT
	Cleanup(func())
}

// newApiQuerierMock creates a new instance of apiQuerierMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func newApiQuerierMock(t mockConstructorTestingTnewApiQuerierMock) *apiQuerierMock {
	mock := &apiQuerierMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
