package environment

import (
	"context"
	"os"
	"testing"
	"time"

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
	s.ExpectedGoroutineIncrease++ // We don't care about removing the created environment.
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
	s.ExpectedGoroutineIncrease++ // We dont care about removing the created environment at this point.
	_, err := s.manager.CreateOrUpdate(
		dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request, context.Background())
	s.NoError(err)
	s.Greater(count, 1)
}

func (s *MainTestSuite) TestNewNomadEnvironmentManager() {
	executorAPIMock := &nomad.ExecutorAPIMock{}
	executorAPIMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
	executorAPIMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	executorAPIMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	runnerManagerMock := &runner.ManagerMock{}
	runnerManagerMock.On("Load").Return()

	previousTemplateEnvironmentJobHCL := templateEnvironmentJobHCL

	s.Run("returns error if template file does not exist", func() {
		_, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, "/non-existent/file")
		s.Error(err)
	})

	s.Run("loads template environment job from file", func() {
		templateJobHCL := "job \"" + tests.DefaultTemplateJobID + "\" {}"

		environment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, executorAPIMock, templateJobHCL)
		s.Require().NoError(err)
		f := createTempFile(s.T(), templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		s.Require().NoError(err)
		s.NotNil(m)
		s.Equal(templateJobHCL, m.templateEnvironmentHCL)

		s.NoError(environment.Delete(tests.ErrCleanupDestroyReason))
	})

	s.Run("returns error if template file is invalid", func() {
		templateJobHCL := "invalid hcl file"
		f := createTempFile(s.T(), templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		s.Require().NoError(err)
		_, err = NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil, m.templateEnvironmentHCL)
		s.Error(err)
	})

	templateEnvironmentJobHCL = previousTemplateEnvironmentJobHCL
}

func (s *MainTestSuite) TestNomadEnvironmentManager_Get() {
	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(s.TestCtx, apiMock)
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	environmentManager, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
	s.Require().NoError(err)

	s.Run("Returns error when not found", func() {
		_, err := environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, false)
		s.Error(err)
	})

	s.Run("Returns environment when it was added before", func() {
		expectedEnvironment, err :=
			NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		expectedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		s.Require().NoError(err)
		runnerManager.StoreEnvironment(expectedEnvironment)

		environment, err := environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, false)
		s.Require().NoError(err)
		s.Equal(expectedEnvironment, environment)

		err = environment.Delete(tests.ErrCleanupDestroyReason)
		s.Require().NoError(err)
	})

	s.Run("Fetch", func() {
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		s.Run("Returns error when not found", func() {
			_, err := environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, true)
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

			environment, err := environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, false)
			s.NoError(err)
			s.NotEqual(fetchedEnvironment.Image(), environment.Image())

			environment, err = environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, true)
			s.NoError(err)
			s.Equal(fetchedEnvironment.Image(), environment.Image())

			err = fetchedEnvironment.Delete(tests.ErrCleanupDestroyReason)
			s.Require().NoError(err)
			err = environment.Delete(tests.ErrCleanupDestroyReason)
			s.Require().NoError(err)
			err = localEnvironment.Delete(tests.ErrCleanupDestroyReason)
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

			_, err = environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, false)
			s.Require().Error(err)

			environment, err := environmentManager.Get(tests.DefaultEnvironmentIDAsInteger, true)
			s.Require().NoError(err)
			s.Equal(fetchedEnvironment.Image(), environment.Image())

			err = fetchedEnvironment.Delete(tests.ErrCleanupDestroyReason)
			s.Require().NoError(err)
			err = environment.Delete(tests.ErrCleanupDestroyReason)
			s.Require().NoError(err)
		})
	})
}

func (s *MainTestSuite) TestNomadEnvironmentManager_List() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	mockWatchAllocations(s.TestCtx, apiMock)
	call := apiMock.On("LoadEnvironmentJobs")
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]*nomadApi.Job{}, nil}
	})

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	environmentManager, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
	s.Require().NoError(err)

	s.Run("with no environments", func() {
		environments, err := environmentManager.List(true)
		s.NoError(err)
		s.Empty(environments)
	})

	s.Run("Returns added environment", func() {
		localEnvironment, err := NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, apiMock, templateEnvironmentJobHCL)
		s.Require().NoError(err)
		localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(localEnvironment)

		environments, err := environmentManager.List(false)
		s.NoError(err)
		s.Len(environments, 1)
		s.Equal(localEnvironment, environments[0])

		err = localEnvironment.Delete(tests.ErrCleanupDestroyReason)
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

		environments, err := environmentManager.List(false)
		s.NoError(err)
		s.Empty(environments)

		environments, err = environmentManager.List(true)
		s.NoError(err)
		s.Len(environments, 1)
		nomadEnvironment, ok := environments[0].(*NomadEnvironment)
		s.True(ok)
		s.Equal(fetchedEnvironment.job, nomadEnvironment.job)

		err = fetchedEnvironment.Delete(tests.ErrCleanupDestroyReason)
		s.Require().NoError(err)
		err = nomadEnvironment.Delete(tests.ErrCleanupDestroyReason)
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

	s.Run("deletes local environments before loading Nomad environments", func() {
		call.Return([]*nomadApi.Job{}, nil)
		environment := &runner.ExecutionEnvironmentMock{}
		environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
		environment.On("Image").Return("")
		environment.On("CPULimit").Return(uint(0))
		environment.On("MemoryLimit").Return(uint(0))
		environment.On("NetworkAccess").Return(false, nil)
		environment.On("Delete", mock.Anything).Return(nil)
		runnerManager.StoreEnvironment(environment)

		m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
		s.Require().NoError(err)

		err = m.load()
		s.Require().NoError(err)
		environment.AssertExpectations(s.T())
	})
	runnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)

	s.Run("Stores fetched environments", func() {
		_, job := helpers.CreateTemplateJob()
		call.Return([]*nomadApi.Job{job}, nil)

		_, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().False(ok)

		m, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
		s.Require().NoError(err)

		err = m.load()
		s.Require().NoError(err)

		environment, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().True(ok)
		s.Equal("python:latest", environment.Image())

		err = environment.Delete(tests.ErrCleanupDestroyReason)
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

		_, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
		s.Require().NoError(err)

		_, ok = runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
		s.Require().False(ok)
	})
}

func (s *MainTestSuite) TestNomadEnvironmentManager_KeepEnvironmentsSynced() {
	apiMock := &nomad.ExecutorAPIMock{}
	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	environmentManager, err := NewNomadEnvironmentManager(runnerManager, apiMock, "")
	s.Require().NoError(err)

	s.Run("stops when context is done", func() {
		apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, context.DeadlineExceeded)
		ctx, cancel := context.WithCancel(s.TestCtx)
		cancel()

		var done bool
		go func() {
			<-time.After(tests.ShortTimeout)
			if !done {
				s.Fail("KeepEnvironmentsSynced is ignoring the context")
			}
		}()

		environmentManager.KeepEnvironmentsSynced(func(_ context.Context) error { return nil }, ctx)
		done = true
	})
	apiMock.ExpectedCalls = []*mock.Call{}
	apiMock.Calls = []mock.Call{}

	s.Run("retries loading environments", func() {
		ctx, cancel := context.WithCancel(s.TestCtx)

		apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, context.DeadlineExceeded).Once()
		apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil).Run(func(_ mock.Arguments) {
			cancel()
		}).Once()

		environmentManager.KeepEnvironmentsSynced(func(_ context.Context) error { return nil }, ctx)
		apiMock.AssertExpectations(s.T())
	})
	apiMock.ExpectedCalls = []*mock.Call{}
	apiMock.Calls = []mock.Call{}

	s.Run("retries synchronizing runners", func() {
		apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
		ctx, cancel := context.WithCancel(s.TestCtx)

		count := 0
		synchronizeRunners := func(ctx context.Context) error {
			count++
			if count >= 2 {
				cancel()
				return nil
			}
			return context.DeadlineExceeded
		}
		environmentManager.KeepEnvironmentsSynced(synchronizeRunners, ctx)

		if count < 2 {
			s.Fail("KeepEnvironmentsSynced is not retrying to synchronize the runners")
		}
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
	file, err := os.CreateTemp("", "test")
	require.NoError(t, err)
	n, err := file.WriteString(content)
	require.NoError(t, err)
	require.Equal(t, len(content), n)

	return file
}
