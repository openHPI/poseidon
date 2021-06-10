// Code generated by mockery v2.8.0. DO NOT EDIT.

package environment

import (
	mock "github.com/stretchr/testify/mock"
	dto "gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"

	runner "gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
)

// ManagerMock is an autogenerated mock type for the Manager type
type ManagerMock struct {
	mock.Mock
}

// CreateOrUpdate provides a mock function with given fields: id, request
func (_m *ManagerMock) CreateOrUpdate(id runner.EnvironmentID, request dto.ExecutionEnvironmentRequest) (bool, error) {
	ret := _m.Called(id, request)

	var r0 bool
	if rf, ok := ret.Get(0).(func(runner.EnvironmentID, dto.ExecutionEnvironmentRequest) bool); ok {
		r0 = rf(id, request)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(runner.EnvironmentID, dto.ExecutionEnvironmentRequest) error); ok {
		r1 = rf(id, request)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: id
func (_m *ManagerMock) Delete(id string) {
	_m.Called(id)
}
