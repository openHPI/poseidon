package environment

import (
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

type AWSEnvironment struct {
	id dto.EnvironmentID
}

func NewAWSEnvironment() *AWSEnvironment {
	return &AWSEnvironment{}
}

func (a *AWSEnvironment) MarshalJSON() ([]byte, error) {
	panic("implement me")
}

func (a *AWSEnvironment) ID() dto.EnvironmentID {
	return a.id
}

func (a *AWSEnvironment) SetID(id dto.EnvironmentID) {
	a.id = id
}

func (a *AWSEnvironment) PrewarmingPoolSize() uint {
	panic("implement me")
}

func (a *AWSEnvironment) SetPrewarmingPoolSize(_ uint) {
	panic("implement me")
}

func (a *AWSEnvironment) ApplyPrewarmingPoolSize() error {
	panic("implement me")
}

func (a *AWSEnvironment) CPULimit() uint {
	panic("implement me")
}

func (a *AWSEnvironment) SetCPULimit(_ uint) {
	panic("implement me")
}

func (a *AWSEnvironment) MemoryLimit() uint {
	panic("implement me")
}

func (a *AWSEnvironment) SetMemoryLimit(_ uint) {
	panic("implement me")
}

func (a *AWSEnvironment) Image() string {
	panic("implement me")
}

func (a *AWSEnvironment) SetImage(_ string) {
	panic("implement me")
}

func (a *AWSEnvironment) NetworkAccess() (enabled bool, mappedPorts []uint16) {
	panic("implement me")
}

func (a *AWSEnvironment) SetNetworkAccess(_ bool, _ []uint16) {
	panic("implement me")
}

func (a *AWSEnvironment) SetConfigFrom(_ runner.ExecutionEnvironment) {
	panic("implement me")
}

func (a *AWSEnvironment) Register() error {
	panic("implement me")
}

func (a *AWSEnvironment) Delete() error {
	panic("implement me")
}

func (a *AWSEnvironment) Sample() (r runner.Runner, ok bool) {
	panic("implement me")
}

func (a *AWSEnvironment) AddRunner(_ runner.Runner) {
	panic("implement me")
}

func (a *AWSEnvironment) DeleteRunner(_ string) {
	panic("implement me")
}

func (a *AWSEnvironment) IdleRunnerCount() int {
	panic("implement me")
}
