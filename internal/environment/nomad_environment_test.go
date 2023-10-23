package environment

import (
	"context"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

func (s *MainTestSuite) TestConfigureNetworkCreatesNewNetworkWhenNoNetworkExists() {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	environment := &NomadEnvironment{nil, "", job, nil, context.Background(), nil}

	if s.Equal(0, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		s.Equal(1, len(defaultTaskGroup.Networks))
	}
}

func (s *MainTestSuite) TestConfigureNetworkDoesNotCreateNewNetworkWhenNetworkExists() {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	environment := &NomadEnvironment{nil, "", job, nil, context.Background(), nil}

	networkResource := &nomadApi.NetworkResource{Mode: "cni/secure-bridge"}
	defaultTaskGroup.Networks = []*nomadApi.NetworkResource{networkResource}

	if s.Equal(1, len(defaultTaskGroup.Networks)) {
		environment.SetNetworkAccess(true, []uint16{})

		s.Equal(1, len(defaultTaskGroup.Networks))
		s.Equal(networkResource, defaultTaskGroup.Networks[0])
	}
}

func (s *MainTestSuite) TestConfigureNetworkSetsCorrectValues() {
	_, job := helpers.CreateTemplateJob()
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

	mode, ok := defaultTask.Config["network_mode"]
	s.True(ok)
	s.Equal("none", mode)
	s.Equal(0, len(defaultTaskGroup.Networks))

	exposedPortsTests := [][]uint16{{}, {1337}, {42, 1337}}
	s.Run("with no network access", func() {
		for _, ports := range exposedPortsTests {
			_, testJob := helpers.CreateTemplateJob()
			testTaskGroup := nomad.FindAndValidateDefaultTaskGroup(testJob)
			testTask := nomad.FindAndValidateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{nil, "", job, nil, context.Background(), nil}

			testEnvironment.SetNetworkAccess(false, ports)
			mode, ok := testTask.Config["network_mode"]
			s.True(ok)
			s.Equal("none", mode)
			s.Equal(0, len(testTaskGroup.Networks))
		}
	})

	s.Run("with network access", func() {
		for _, ports := range exposedPortsTests {
			_, testJob := helpers.CreateTemplateJob()
			testTaskGroup := nomad.FindAndValidateDefaultTaskGroup(testJob)
			testTask := nomad.FindAndValidateDefaultTask(testTaskGroup)
			testEnvironment := &NomadEnvironment{nil, "", testJob, nil, context.Background(), nil}

			testEnvironment.SetNetworkAccess(true, ports)
			s.Require().Equal(1, len(testTaskGroup.Networks))

			networkResource := testTaskGroup.Networks[0]
			s.Equal("cni/secure-bridge", networkResource.Mode)
			s.Require().Equal(len(ports), len(networkResource.DynamicPorts))

			assertExpectedPorts(s.T(), ports, networkResource)

			mode, ok := testTask.Config["network_mode"]
			s.True(ok)
			s.Equal(mode, "")
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

func (s *MainTestSuite) TestRegisterFailsWhenNomadJobRegistrationFails() {
	apiClientMock := &nomad.ExecutorAPIMock{}
	expectedErr := tests.ErrDefault

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", expectedErr)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), nil, nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	s.ErrorIs(err, expectedErr)
	apiClientMock.AssertNotCalled(s.T(), "MonitorEvaluation")
}

func (s *MainTestSuite) TestRegisterTemplateJobSucceedsWhenMonitoringEvaluationSucceeds() {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), context.Background(), nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	s.NoError(err)
}

func (s *MainTestSuite) TestRegisterTemplateJobReturnsErrorWhenMonitoringEvaluationFails() {
	apiClientMock := &nomad.ExecutorAPIMock{}
	evaluationID := "id"

	apiClientMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiClientMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(tests.ErrDefault)
	apiClientMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiClientMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	environment := &NomadEnvironment{apiClientMock, "", &nomadApi.Job{},
		storage.NewLocalStorage[runner.Runner](), context.Background(), nil}
	environment.SetID(tests.DefaultEnvironmentIDAsInteger)
	err := environment.Register()

	s.ErrorIs(err, tests.ErrDefault)
}

func (s *MainTestSuite) TestParseJob() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.Run("parses the given default job", func() {
		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		s.NoError(err)
		s.NotNil(environment.job)
		s.NoError(environment.Delete(tests.ErrCleanupDestroyReason))
	})

	s.Run("returns error when given wrong job", func() {
		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, "")
		s.Error(err)
		s.Nil(environment)
	})
}

func (s *MainTestSuite) TestTwoSampleAddExactlyTwoRunners() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job")).Return(nil)

	_, job := helpers.CreateTemplateJob()
	environment := &NomadEnvironment{apiMock, templateEnvironmentJobHCL, job,
		storage.NewLocalStorage[runner.Runner](), context.Background(), nil}
	environment.SetPrewarmingPoolSize(2)
	runner1 := &runner.RunnerMock{}
	runner1.On("ID").Return(tests.DefaultRunnerID)
	runner2 := &runner.RunnerMock{}
	runner2.On("ID").Return(tests.AnotherRunnerID)

	environment.AddRunner(runner1)
	environment.AddRunner(runner2)

	_, ok := environment.Sample()
	s.Require().True(ok)
	_, ok = environment.Sample()
	s.Require().True(ok)

	<-time.After(tests.ShortTimeout) // New Runners are requested asynchronously
	apiMock.AssertNumberOfCalls(s.T(), "RegisterRunnerJob", 2)
}

func (s *MainTestSuite) TestSampleDoesNotSetForcePullFlag() {
	apiMock := &nomad.ExecutorAPIMock{}
	call := apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job"))
	call.Run(func(args mock.Arguments) {
		job, ok := args.Get(0).(*nomadApi.Job)
		s.True(ok)

		taskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
		task := nomad.FindAndValidateDefaultTask(taskGroup)
		s.False(task.Config["force_pull"].(bool))

		call.ReturnArguments = mock.Arguments{nil}
	})

	_, job := helpers.CreateTemplateJob()
	environment := &NomadEnvironment{apiMock, templateEnvironmentJobHCL, job,
		storage.NewLocalStorage[runner.Runner](), s.TestCtx, nil}
	runner1 := &runner.RunnerMock{}
	runner1.On("ID").Return(tests.DefaultRunnerID)
	environment.AddRunner(runner1)

	_, ok := environment.Sample()
	s.Require().True(ok)
	<-time.After(tests.ShortTimeout) // New Runners are requested asynchronously
}

func (s *MainTestSuite) TestNomadEnvironment_DeleteLocally() {
	apiMock := &nomad.ExecutorAPIMock{}
	environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
	s.Require().NoError(err)

	err = environment.Delete(runner.ErrLocalDestruction)
	s.NoError(err)
	apiMock.AssertExpectations(s.T())
}
