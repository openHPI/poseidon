package environment

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
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
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(job)
	environment := &NomadEnvironment{"", job, nil}

	if assert.Equal(t, 0, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		assert.Equal(t, 1, len(defaultTaskGroup.Networks))
	}
}

func TestConfigureNetworkDoesNotCreateNewNetworkWhenNetworkExists(t *testing.T) {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(job)
	environment := &NomadEnvironment{"", job, nil}

	networkResource := &nomadApi.NetworkResource{Mode: "bridge"}
	defaultTaskGroup.Networks = []*nomadApi.NetworkResource{networkResource}

	if assert.Equal(t, 1, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		assert.Equal(t, 1, len(defaultTaskGroup.Networks))
		assert.Equal(t, networkResource, defaultTaskGroup.Networks[0])
	}
}

func TestConfigureNetworkSetsCorrectValues(t *testing.T) {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)

	mode, ok := defaultTask.Config["network_mode"]
	assert.True(t, ok)
	assert.Equal(t, "none", mode)
	assert.Equal(t, 0, len(defaultTaskGroup.Networks))

	exposedPortsTests := [][]uint16{{}, {1337}, {42, 1337}}
	t.Run("with no network access", func(t *testing.T) {
		for _, ports := range exposedPortsTests {
			_, testJob := helpers.CreateTemplateJob()
			testTaskGroup := nomad.FindOrCreateDefaultTaskGroup(testJob)
			testTask := nomad.FindOrCreateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{"", job, nil}

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
			testTaskGroup := nomad.FindOrCreateDefaultTaskGroup(testJob)
			testTask := nomad.FindOrCreateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{"", testJob, nil}

			testEnvironment.SetNetworkAccess(true, ports)
			require.Equal(t, 1, len(testTaskGroup.Networks))

			networkResource := testTaskGroup.Networks[0]
			assert.Equal(t, "bridge", networkResource.Mode)
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

	environment := &NomadEnvironment{"", &nomadApi.Job{}, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register(apiClientMock)

	assert.ErrorIs(t, err, expectedErr)
	apiClientMock.AssertNotCalled(t, "EvaluationStream")
}

func TestRegisterTemplateJobSucceedsWhenMonitoringEvaluationSucceeds(t *testing.T) {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	stream := make(chan *nomadApi.Events)
	readonlyStream := func() <-chan *nomadApi.Events {
		return stream
	}()
	// Immediately close stream to avoid any reading from it resulting in endless wait
	close(stream)

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	apiClientMock.On("EvaluationStream", evaluationID, mock.AnythingOfType("*context.emptyCtx")).
		Return(readonlyStream, nil)

	environment := &NomadEnvironment{"", &nomadApi.Job{}, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register(apiClientMock)

	assert.NoError(t, err)
}

func TestRegisterTemplateJobReturnsErrorWhenMonitoringEvaluationFails(t *testing.T) {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(tests.ErrDefault)

	environment := &NomadEnvironment{"", &nomadApi.Job{}, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register(apiClientMock)

	assert.ErrorIs(t, err, tests.ErrDefault)
}

func TestParseJob(t *testing.T) {
	t.Run("parses the given default job", func(t *testing.T) {
		environment, err := NewNomadEnvironment(templateEnvironmentJobHCL)
		assert.NoError(t, err)
		assert.NotNil(t, environment.job)
	})

	t.Run("returns error when given wrong job", func(t *testing.T) {
		environment, err := NewNomadEnvironment("")
		assert.Error(t, err)
		assert.Nil(t, environment)
	})
}

func TestTwoSampleAddExactlyTwoRunners(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job")).Return(nil)

	_, job := helpers.CreateTemplateJob()
	environment := &NomadEnvironment{templateEnvironmentJobHCL, job, runner.NewLocalRunnerStorage()}
	runner1 := &runner.RunnerMock{}
	runner1.On("ID").Return(tests.DefaultRunnerID)
	runner2 := &runner.RunnerMock{}
	runner2.On("ID").Return(tests.AnotherRunnerID)

	environment.AddRunner(runner1)
	environment.AddRunner(runner2)

	_, ok := environment.Sample(apiMock)
	require.True(t, ok)
	_, ok = environment.Sample(apiMock)
	require.True(t, ok)

	<-time.After(tests.ShortTimeout) // New Runners are requested asynchronously
	apiMock.AssertNumberOfCalls(t, "RegisterRunnerJob", 2)
}
