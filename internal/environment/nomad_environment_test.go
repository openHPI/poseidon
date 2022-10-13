package environment

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfigureNetworkCreatesNewNetworkWhenNoNetworkExists(t *testing.T) {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	environment := &NomadEnvironment{nil, "", job, nil, nil, nil}

	if assert.Equal(t, 0, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		assert.Equal(t, 1, len(defaultTaskGroup.Networks))
	}
}

func TestConfigureNetworkDoesNotCreateNewNetworkWhenNetworkExists(t *testing.T) {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	environment := &NomadEnvironment{nil, "", job, nil, nil, nil}

	networkResource := &nomadApi.NetworkResource{Mode: "cni/secure-bridge"}
	defaultTaskGroup.Networks = []*nomadApi.NetworkResource{networkResource}

	if assert.Equal(t, 1, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		assert.Equal(t, 1, len(defaultTaskGroup.Networks))
		assert.Equal(t, networkResource, defaultTaskGroup.Networks[0])
	}
}

func TestConfigureNetworkSetsCorrectValues(t *testing.T) {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

	mode, ok := defaultTask.Config["network_mode"]
	assert.True(t, ok)
	assert.Equal(t, "none", mode)
	assert.Equal(t, 0, len(defaultTaskGroup.Networks))

	exposedPortsTests := [][]uint16{{}, {1337}, {42, 1337}}
	t.Run("with no network access", func(t *testing.T) {
		for _, ports := range exposedPortsTests {
			_, testJob := helpers.CreateTemplateJob()
			testTaskGroup := nomad.FindAndValidateDefaultTaskGroup(testJob)
			testTask := nomad.FindAndValidateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{nil, "", job, nil, nil, nil}

			testEnvironment.SetNetworkAccess(false, ports)
			mode, ok := testTask.Config["network_mode"]
			assert.True(t, ok)
			assert.Equal(t, "none", mode)
			assert.Equal(t, 0, len(testTaskGroup.Networks))
		}
	})

	t.Run("with network access", func(t *testing.T) {
		for _, ports := range exposedPortsTests {
			_, testJob := helpers.CreateTemplateJob()
			testTaskGroup := nomad.FindAndValidateDefaultTaskGroup(testJob)
			testTask := nomad.FindAndValidateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{nil, "", testJob, nil, nil, nil}

			testEnvironment.SetNetworkAccess(true, ports)
			require.Equal(t, 1, len(testTaskGroup.Networks))

			networkResource := testTaskGroup.Networks[0]
			assert.Equal(t, "cni/secure-bridge", networkResource.Mode)
			require.Equal(t, len(ports), len(networkResource.DynamicPorts))

			assertExpectedPorts(t, ports, networkResource)

			mode, ok := testTask.Config["network_mode"]
			assert.True(t, ok)
			assert.Equal(t, mode, "")
		}
	})
}

func assertExpectedPorts(t *testing.T, expectedPorts []uint16, networkResource *nomadApi.NetworkResource) {
	t.Helper()
	for _, expectedPort := range expectedPorts {
		found := false
		for _, actualPort := range networkResource.DynamicPorts {
			if actualPort.To == int(expectedPort) {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("port list should contain %v", expectedPort))
	}
}

func TestRegisterFailsWhenNomadJobRegistrationFails(t *testing.T) {
	apiClientMock := &nomad.ExecutorAPIMock{}
	expectedErr := tests.ErrDefault

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", expectedErr)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	assert.ErrorIs(t, err, expectedErr)
	apiClientMock.AssertNotCalled(t, "MonitorEvaluation")
}

func TestRegisterTemplateJobSucceedsWhenMonitoringEvaluationSucceeds(t *testing.T) {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	assert.NoError(t, err)
}

func TestRegisterTemplateJobReturnsErrorWhenMonitoringEvaluationFails(t *testing.T) {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(tests.ErrDefault)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	assert.ErrorIs(t, err, tests.ErrDefault)
}

func TestParseJob(t *testing.T) {
	t.Run("parses the given default job", func(t *testing.T) {
		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, templateEnvironmentJobHCL)
		assert.NoError(t, err)
		assert.NotNil(t, environment.job)
	})

	t.Run("returns error when given wrong job", func(t *testing.T) {
		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, "")
		assert.Error(t, err)
		assert.Nil(t, environment)
	})
}

func TestTwoSampleAddExactlyTwoRunners(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job")).Return(nil)

	_, job := helpers.CreateTemplateJob()
	environment := &NomadEnvironment{apiMock, templateEnvironmentJobHCL, job,
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	runner1 := &runner.RunnerMock{}
	runner1.On("ID").Return(tests.DefaultRunnerID)
	runner2 := &runner.RunnerMock{}
	runner2.On("ID").Return(tests.AnotherRunnerID)

	environment.AddRunner(runner1)
	environment.AddRunner(runner2)

	_, ok := environment.Sample()
	require.True(t, ok)
	_, ok = environment.Sample()
	require.True(t, ok)

	<-time.After(tests.ShortTimeout) // New Runners are requested asynchronously
	apiMock.AssertNumberOfCalls(t, "RegisterRunnerJob", 2)
}

func TestSampleDoesNotSetForcePullFlag(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	call := apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job"))
	call.Run(func(args mock.Arguments) {
		job, ok := args.Get(0).(*nomadApi.Job)
		assert.True(t, ok)

		taskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
		task := nomad.FindAndValidateDefaultTask(taskGroup)
		assert.False(t, task.Config["force_pull"].(bool))

		call.ReturnArguments = mock.Arguments{nil}
	})

	_, job := helpers.CreateTemplateJob()
	environment := &NomadEnvironment{apiMock, templateEnvironmentJobHCL, job,
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	runner1 := &runner.RunnerMock{}
	runner1.On("ID").Return(tests.DefaultRunnerID)
	environment.AddRunner(runner1)

	_, ok := environment.Sample()
	require.True(t, ok)
	<-time.After(tests.ShortTimeout) // New Runners are requested asynchronously
}
