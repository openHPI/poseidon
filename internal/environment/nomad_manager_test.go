package environment

import (
	"context"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
)

type CreateOrUpdateTestSuite struct {
	tests.MemoryLeakTestSuite
	runnerManagerMock runner.ManagerMock
	apiMock           nomad.ExecutorAPIMock
	request           dto.ExecutionEnvironmentRequest
	manager           *NomadEnvironmentManager
	environmentID     dto.EnvironmentID
}

func TestCreateOrUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(CreateOrUpdateTestSuite))
}

func (s *CreateOrUpdateTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.runnerManagerMock = runner.ManagerMock{}

	s.apiMock = nomad.ExecutorAPIMock{}
	s.request = dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 10,
		CPULimit:           20,
		MemoryLimit:        30,
		Image:              "my-image",
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	s.manager = &NomadEnvironmentManager{
		AbstractManager:        &AbstractManager{runnerManager: &s.runnerManagerMock},
		api:                    &s.apiMock,
		templateEnvironmentHCL: templateEnvironmentJobHCL,
	}

	s.environmentID = dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger)
}

func (s *CreateOrUpdateTestSuite) TestReturnsErrorIfCreatesOrUpdateEnvironmentReturnsError() {
	s.apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", tests.ErrDefault)
	s.apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.runnerManagerMock.On("GetEnvironment", mock.AnythingOfType("dto.EnvironmentID")).Return(nil, false)
	s.runnerManagerMock.On("StoreEnvironment", mock.AnythingOfType("*environment.NomadEnvironment")).Return(true)
	s.ExpectedGoroutingIncrease++ // We don't care about removing the created environment.
	_, err := s.manager.CreateOrUpdate(
		dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request, context.Background())
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *CreateOrUpdateTestSuite) TestCreateOrUpdatesSetsForcePullFlag() {
	s.apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", nil)
	s.apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.runnerManagerMock.On("GetEnvironment", mock.AnythingOfType("dto.EnvironmentID")).Return(nil, false)
	s.runnerManagerMock.On("StoreEnvironment", mock.AnythingOfType("*environment.NomadEnvironment")).Return(true)
	s.apiMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	s.apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	call := s.apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job"))
	count := 0
	call.Run(func(args mock.Arguments) {
		count++
		job, ok := args.Get(0).(*nomadApi.Job)
		s.True(ok)

		// The environment job itself has not the force_pull flag
		if count > 1 {
			taskGroup := nomad.FindAndValidateDefaultTaskGroup(job)
			task := nomad.FindAndValidateDefaultTask(taskGroup)
			s.True(task.Config["force_pull"].(bool))
		}

		call.ReturnArguments = mock.Arguments{nil}
	})
	s.ExpectedGoroutingIncrease++ // We dont care about removing the created environment at this point.
	_, err := s.manager.CreateOrUpdate(
		dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request, context.Background())
	s.NoError(err)
	s.True(count > 1)
}

func (s *MainTestSuite) TestNewNomadEnvironmentManager() {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	executorAPIMock := &nomad.ExecutorAPIMock{}
	executorAPIMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
	executorAPIMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	executorAPIMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	runnerManagerMock := &runner.ManagerMock{}
	runnerManagerMock.On("Load").Return()

	previousTemplateEnvironmentJobHCL := templateEnvironmentJobHCL

	s.Run("returns error if template file does not exist", func() {
		_, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, "/non-existent/file", disableRecovery)
		s.Error(err)
	})

	s.Run("loads template environment job from file", func() {
		templateJobHCL := "job \"" + tests.DefaultTemplateJobID + "\" {}"

		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, executorAPIMock, templateJobHCL)
		s.Require().NoError(err)
		f := createTempFile(s.T(), templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name(), disableRecovery)
		s.NoError(err)
		s.NotNil(m)
		s.Equal(templateJobHCL, m.templateEnvironmentHCL)

		s.NoError(environment.Delete(false))
	})

	s.Run("returns error if template file is invalid", func() {
		templateJobHCL := "invalid hcl file"
		f := createTempFile(s.T(), templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name(), disableRecovery)
		s.Require().NoError(err)
		_, err = NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, m.templateEnvironmentHCL)
		s.Error(err)
	})

	templateEnvironmentJobHCL = previousTemplateEnvironmentJobHCL
}

func (s *MainTestSuite) TestNomadEnvironmentManager_Get() {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(s.TestCtx, apiMock)
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", disableRecovery)
	s.Require().NoError(err)

	s.Run("Returns error when not found", func() {
		_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		s.Error(err)
	})

	s.Run("Returns environment when it was added before", func() {
		expectedEnvironment, err :=
			NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		expectedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		s.Require().NoError(err)
		runnerManager.StoreEnvironment(expectedEnvironment)

		environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		s.NoError(err)
		s.Equal(expectedEnvironment, environment)

		err = environment.Delete(false)
		s.Require().NoError(err)
	})

	s.Run("Fetch", func() {
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		s.Run("Returns error when not found", func() {
			_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			s.Error(err)
		})

		s.Run("Updates values when environment already known by Poseidon", func() {
			fetchedEnvironment, err := NewNomadEnvironment(
				tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
			s.Require().NoError(err)
			fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			fetchedEnvironment.SetImage("random docker image")
			call.Run(func(args mock.Arguments) {
				call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
			})

			localEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
			s.Require().NoError(err)
			localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			runnerManager.StoreEnvironment(localEnvironment)

			environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
			s.NoError(err)
			s.NotEqual(fetchedEnvironment.Image(), environment.Image())

			environment, err = m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			s.NoError(err)
			s.Equal(fetchedEnvironment.Image(), environment.Image())

			err = fetchedEnvironment.Delete(false)
			s.Require().NoError(err)
			err = environment.Delete(false)
			s.Require().NoError(err)
			err = localEnvironment.Delete(false)
			s.Require().NoError(err)
		})
		runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)

		s.Run("Adds environment when not already known by Poseidon", func() {
			fetchedEnvironment, err := NewNomadEnvironment(
				tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
			s.Require().NoError(err)
			fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			fetchedEnvironment.SetImage("random docker image")
			call.Run(func(args mock.Arguments) {
				call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
			})

			_, err = m.Get(tests.DefaultEnvironmentIDAsInteger, false)
			s.Error(err)

			environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			s.NoError(err)
			s.Equal(fetchedEnvironment.Image(), environment.Image())

			err = fetchedEnvironment.Delete(false)
			s.Require().NoError(err)
			err = environment.Delete(false)
			s.Require().NoError(err)
		})
	})
}

func (s *MainTestSuite) TestNomadEnvironmentManager_List() {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	mockWatchAllocations(s.TestCtx, apiMock)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", disableRecovery)
	s.Require().NoError(err)

	s.Run("with no environments", func() {
		environments, err := m.List(true)
		s.NoError(err)
		s.Empty(environments)
	})

	s.Run("Returns added environment", func() {
		localEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		s.Require().NoError(err)
		localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(localEnvironment)

		environments, err := m.List(false)
		s.NoError(err)
		s.Equal(1, len(environments))
		s.Equal(localEnvironment, environments[0])

		err = localEnvironment.Delete(false)
		s.Require().NoError(err)
	})
	runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)

	s.Run("Fetches new Runners via the api client", func() {
		fetchedEnvironment, err :=
			NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		s.Require().NoError(err)
		fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		status := structs.JobStatusRunning
		fetchedEnvironment.job.Status = &status
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
		})

		environments, err := m.List(false)
		s.NoError(err)
		s.Empty(environments)

		environments, err = m.List(true)
		s.NoError(err)
		s.Equal(1, len(environments))
		nomadEnvironment, ok := environments[0].(*NomadEnvironment)
		s.True(ok)
		s.Equal(fetchedEnvironment.job, nomadEnvironment.job)

		err = fetchedEnvironment.Delete(false)
		s.Require().NoError(err)
		err = nomadEnvironment.Delete(false)
		s.Require().NoError(err)
	})
}

func (s *MainTestSuite) TestNomadEnvironmentManager_Load() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	mockWatchAllocations(s.TestCtx, apiMock)
	call := apiMock.On("LoadEnvironmentJobs")
	apiMock.On("LoadRunnerJobs", mock.AnythingOfType("dto.EnvironmentID")).
		Return([]*nomadApi.Job{}, nil)

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)

	s.Run("Stores fetched environments", func() {
		_, job := helpers.CreateTemplateJob()
		call.Return([]*nomadApi.Job{job}, nil)

		_, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().False(ok)

		_, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", s.TestCtx)
		s.Require().NoError(err)

		environment, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().True(ok)
		s.Equal("python:latest", environment.Image())

		err = environment.Delete(false)
		s.Require().NoError(err)
	})

	runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)
	s.Run("Processes only running environments", func() {
		_, job := helpers.CreateTemplateJob()
		jobStatus := structs.JobStatusDead
		job.Status = &jobStatus
		call.Return([]*nomadApi.Job{job}, nil)

		_, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().False(ok)

		_, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", context.Background())
		s.Require().NoError(err)

		_, ok = runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().False(ok)
	})
}

func mockWatchAllocations(ctx context.Context, apiMock *nomad.ExecutorAPIMock) {
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-ctx.Done()
		call.ReturnArguments = mock.Arguments{nil}
	})
}

func createTempFile(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "test")
	require.NoError(t, err)
	n, err := f.WriteString(content)
	require.NoError(t, err)
	require.Equal(t, len(content), n)

	return f
}
