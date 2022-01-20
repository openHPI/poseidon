package environment

import (
	"encoding/json"
	"fmt"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

type AWSEnvironment struct {
	id          dto.EnvironmentID
	awsEndpoint string
}

func NewAWSEnvironment() *AWSEnvironment {
	return &AWSEnvironment{}
}

func (a *AWSEnvironment) MarshalJSON() ([]byte, error) {
	res, err := json.Marshal(dto.ExecutionEnvironmentData{
		ID:                          int(a.ID()),
		ExecutionEnvironmentRequest: dto.ExecutionEnvironmentRequest{Image: a.Image()},
	})
	if err != nil {
		return res, fmt.Errorf("couldn't marshal aws execution environment: %w", err)
	}
	return res, nil
}

func (a *AWSEnvironment) ID() dto.EnvironmentID {
	return a.id
}

func (a *AWSEnvironment) SetID(id dto.EnvironmentID) {
	a.id = id
}

func (a *AWSEnvironment) PrewarmingPoolSize() uint {
	return 0
}

func (a *AWSEnvironment) SetPrewarmingPoolSize(_ uint) {}

func (a *AWSEnvironment) ApplyPrewarmingPoolSize() error {
	return nil
}

func (a *AWSEnvironment) CPULimit() uint {
	return 0
}

// SetCPULimit is disabled as one can only set the memory limit with AWS Lambda.
func (a *AWSEnvironment) SetCPULimit(_ uint) {}

func (a *AWSEnvironment) MemoryLimit() uint {
	panic("implement me")
}

func (a *AWSEnvironment) SetMemoryLimit(_ uint) {
	panic("implement me")
}

// Image is used to specify the AWS Endpoint Poseidon is connecting to.
func (a *AWSEnvironment) Image() string {
	return a.awsEndpoint
}

func (a *AWSEnvironment) SetImage(awsEndpoint string) {
	a.awsEndpoint = awsEndpoint
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
	workload, err := runner.NewAWSFunctionWorkload(a, nil)
	if err != nil {
		return nil, false
	}
	return workload, true
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
