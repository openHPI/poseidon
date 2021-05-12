// Code generated by mockery v0.0.0-dev. DO NOT EDIT.

package environment

import mock "github.com/stretchr/testify/mock"

// ManagerMock is an autogenerated mock type for the Manager type
type ManagerMock struct {
	mock.Mock
}

// Create provides a mock function with given fields: id, prewarmingPoolSize, cpuLimit, memoryLimit, image, networkAccess, exposedPorts
func (_m *ManagerMock) Create(id string, prewarmingPoolSize uint, cpuLimit uint, memoryLimit uint, image string, networkAccess bool, exposedPorts []uint16) {
	_m.Called(id, prewarmingPoolSize, cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
}

// Delete provides a mock function with given fields: id
func (_m *ManagerMock) Delete(id string) {
	_m.Called(id)
}

// Load provides a mock function with given fields:
func (_m *ManagerMock) Load() {
	_m.Called()
}
