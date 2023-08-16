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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"time"
)

type CreateOrUpdateTestSuite struct {
	suite.Suite
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
	_, err := s.manager.CreateOrUpdate(
		dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request, context.Background())
	s.NoError(err)
	s.True(count > 1)
}

func TestNewNomadEnvironmentManager(t *testing.T) {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	executorAPIMock := &nomad.ExecutorAPIMock{}
	executorAPIMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)

	runnerManagerMock := &runner.ManagerMock{}
	runnerManagerMock.On("Load").Return()

	previousTemplateEnvironmentJobHCL := templateEnvironmentJobHCL

	t.Run("returns error if template file does not exist", func(t *testing.T) {
		_, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, "/non-existent/file", disableRecovery)
		assert.Error(t, err)
	})

	t.Run("loads template environment job from file", func(t *testing.T) {
		templateJobHCL := "job \"" + tests.DefaultTemplateJobID + "\" {}"
		_, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, templateJobHCL)
		require.NoError(t, err)
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name(), disableRecovery)
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, templateJobHCL, m.templateEnvironmentHCL)
	})

	t.Run("returns error if template file is invalid", func(t *testing.T) {
		templateJobHCL := "invalid hcl file"
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name(), disableRecovery)
		require.NoError(t, err)
		_, err = NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, m.templateEnvironmentHCL)
		assert.Error(t, err)
	})

	templateEnvironmentJobHCL = previousTemplateEnvironmentJobHCL
}

func TestNomadEnvironmentManager_Get(t *testing.T) {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(apiMock)
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, context.Background())
	m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", disableRecovery)
	require.NoError(t, err)

	t.Run("Returns error when not found", func(t *testing.T) {
		_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		assert.Error(t, err)
	})

	t.Run("Returns environment when it was added before", func(t *testing.T) {
		expectedEnvironment, err :=
			NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		expectedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		require.NoError(t, err)
		runnerManager.StoreEnvironment(expectedEnvironment)

		environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		assert.NoError(t, err)
		assert.Equal(t, expectedEnvironment, environment)
	})

	t.Run("Fetch", func(t *testing.T) {
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		t.Run("Returns error when not found", func(t *testing.T) {
			_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			assert.Error(t, err)
		})

		t.Run("Updates values when environment already known by Poseidon", func(t *testing.T) {
			fetchedEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, templateEnvironmentJobHCL)
			require.NoError(t, err)
			fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			fetchedEnvironment.SetImage("random docker image")
			call.Run(func(args mock.Arguments) {
				call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
			})

			localEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, templateEnvironmentJobHCL)
			require.NoError(t, err)
			localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			runnerManager.StoreEnvironment(localEnvironment)

			environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
			assert.NoError(t, err)
			assert.NotEqual(t, fetchedEnvironment.Image(), environment.Image())

			environment, err = m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			assert.NoError(t, err)
			assert.Equal(t, fetchedEnvironment.Image(), environment.Image())
		})
		runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)

		t.Run("Adds environment when not already known by Poseidon", func(t *testing.T) {
			fetchedEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, templateEnvironmentJobHCL)
			require.NoError(t, err)
			fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
			fetchedEnvironment.SetImage("random docker image")
			call.Run(func(args mock.Arguments) {
				call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
			})

			_, err = m.Get(tests.DefaultEnvironmentIDAsInteger, false)
			assert.Error(t, err)

			environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, true)
			assert.NoError(t, err)
			assert.Equal(t, fetchedEnvironment.Image(), environment.Image())
		})
	})
}

func TestNomadEnvironmentManager_List(t *testing.T) {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(apiMock)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, context.Background())
	m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", disableRecovery)
	require.NoError(t, err)

	t.Run("with no environments", func(t *testing.T) {
		environments, err := m.List(true)
		assert.NoError(t, err)
		assert.Empty(t, environments)
	})

	t.Run("Returns added environment", func(t *testing.T) {
		localEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		require.NoError(t, err)
		localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(localEnvironment)

		environments, err := m.List(false)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(environments))
		assert.Equal(t, localEnvironment, environments[0])
	})
	runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)

	t.Run("Fetches new Runners via the api client", func(t *testing.T) {
		fetchedEnvironment, err :=
			NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		require.NoError(t, err)
		fetchedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		status := structs.JobStatusRunning
		fetchedEnvironment.job.Status = &status
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{fetchedEnvironment.job}, nil}
		})

		environments, err := m.List(false)
		assert.NoError(t, err)
		assert.Empty(t, environments)

		environments, err = m.List(true)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(environments))
		nomadEnvironment, ok := environments[0].(*NomadEnvironment)
		assert.True(t, ok)
		assert.Equal(t, fetchedEnvironment.job, nomadEnvironment.job)
	})
}

func TestNomadEnvironmentManager_Load(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(apiMock)
	call := apiMock.On("LoadEnvironmentJobs")
	apiMock.On("LoadRunnerJobs", mock.AnythingOfType("dto.EnvironmentID")).
		Return([]*nomadApi.Job{}, nil)

	runnerManager := runner.NewNomadRunnerManager(apiMock, context.Background())

	t.Run("Stores fetched environments", func(t *testing.T) {
		_, job := helpers.CreateTemplateJob()
		call.Return([]*nomadApi.Job{job}, nil)

		_, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		require.False(t, ok)

		_, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", context.Background())
		require.NoError(t, err)

		environment, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		require.True(t, ok)
		assert.Equal(t, "python:latest", environment.Image())
	})

	runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)
	t.Run("Processes only running environments", func(t *testing.T) {
		_, job := helpers.CreateTemplateJob()
		jobStatus := structs.JobStatusDead
		job.Status = &jobStatus
		call.Return([]*nomadApi.Job{job}, nil)

		_, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		require.False(t, ok)

		_, err := NewNomadEnvironmentManager(runnerManager, apiMock, "", context.Background())
		require.NoError(t, err)

		_, ok = runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		require.False(t, ok)
	})
}

func mockWatchAllocations(apiMock *nomad.ExecutorAPIMock) {
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-time.After(tests.DefaultTestTimeout)
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
