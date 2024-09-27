package runner

import (
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestGetNextRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}

type ManagerTestSuite struct {
	tests.MemoryLeakTestSuite
	apiMock             *nomad.ExecutorAPIMock
	nomadRunnerManager  *NomadRunnerManager
	exerciseEnvironment *ExecutionEnvironmentMock
	exerciseRunner      Runner
}

func (s *ManagerTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.apiMock = &nomad.ExecutorAPIMock{}
	mockRunnerQueries(s.TestCtx, s.apiMock, []string{})
	// Instantly closed context to manually start the update process in some cases
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.nomadRunnerManager = NewNomadRunnerManager(ctx, s.apiMock)

	s.exerciseRunner = NewNomadJob(ctx, tests.DefaultRunnerID, nil, s.apiMock, s.nomadRunnerManager.onRunnerDestroyed)
	s.exerciseEnvironment = createBasicEnvironmentMock(defaultEnvironmentID)
	s.nomadRunnerManager.StoreEnvironment(s.exerciseEnvironment)
}

func (s *ManagerTestSuite) TearDownTest() {
	defer s.MemoryLeakTestSuite.TearDownTest()
	err := s.exerciseRunner.Destroy(nil)
	s.Require().NoError(err)
}

func mockRunnerQueries(ctx context.Context, apiMock *nomad.ExecutorAPIMock, returnedRunnerIDs []string) {
	// reset expected calls to allow new mocked return values
	apiMock.ExpectedCalls = []*mock.Call{}
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(_ mock.Arguments) {
		<-ctx.Done()
		call.ReturnArguments = mock.Arguments{nil}
	})
	apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
	apiMock.On("LoadRunnerJobs", mock.AnythingOfType("dto.EnvironmentID")).Return([]*nomadApi.Job{}, nil)
	apiMock.On("SetRunnerMetaUsed", mock.AnythingOfType("string"), mock.AnythingOfType("bool"), mock.AnythingOfType("int")).Return(nil)
	apiMock.On("LoadRunnerIDs", tests.DefaultRunnerID).Return(returnedRunnerIDs, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	apiMock.On("RegisterRunnerJob", mock.Anything).Return(nil)
	apiMock.On("MonitorEvaluation", mock.Anything, mock.Anything).Return(nil)
}

func mockIdleRunners(environmentMock *ExecutionEnvironmentMock) {
	tests.RemoveMethodFromMock(&environmentMock.Mock, "DeleteRunner")
	idleRunner := storage.NewLocalStorage[Runner]()
	environmentMock.On("AddRunner", mock.Anything).Run(func(args mock.Arguments) {
		r, ok := args.Get(0).(Runner)
		if !ok {
			return
		}
		idleRunner.Add(r.ID(), r)
	})
	sampleCall := environmentMock.On("Sample", mock.Anything)
	sampleCall.Run(func(_ mock.Arguments) {
		r, ok := idleRunner.Sample()
		sampleCall.ReturnArguments = mock.Arguments{r, ok}
	})
	deleteCall := environmentMock.On("DeleteRunner", mock.AnythingOfType("string"))
	deleteCall.Run(func(args mock.Arguments) {
		runnerID, ok := args.Get(0).(string)
		if !ok {
			log.Fatal("Cannot parse ID")
		}
		r, ok := idleRunner.Get(runnerID)
		deleteCall.ReturnArguments = mock.Arguments{r, ok}
		if !ok {
			return
		}
		idleRunner.Delete(runnerID)
	})
}

func (s *ManagerTestSuite) waitForRunnerRefresh() {
	<-time.After(tests.ShortTimeout)
}

func (s *ManagerTestSuite) TestSetEnvironmentAddsNewEnvironment() {
	anotherEnvironment := createBasicEnvironmentMock(anotherEnvironmentID)
	s.nomadRunnerManager.StoreEnvironment(anotherEnvironment)

	job, ok := s.nomadRunnerManager.environments.Get(anotherEnvironmentID.ToString())
	s.True(ok)
	s.NotNil(job)
}

func (s *ManagerTestSuite) TestClaimReturnsNotFoundErrorIfEnvironmentNotFound() {
	runner, err := s.nomadRunnerManager.Claim(anotherEnvironmentID, defaultInactivityTimeout)
	s.Nil(runner)
	s.ErrorIs(err, ErrUnknownExecutionEnvironment)
}

func (s *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	s.Equal(s.exerciseRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestClaimReturnsErrorIfNoRunnerAvailable() {
	s.waitForRunnerRefresh()
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(nil, false)
	runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Nil(runner)
	s.Equal(ErrNoRunnersAvailable, err)
}

func (s *ManagerTestSuite) TestClaimReturnsNoRunnerOfDifferentEnvironment() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true)
	receivedRunner, err := s.nomadRunnerManager.Claim(anotherEnvironmentID, defaultInactivityTimeout)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimDoesNotReturnTheSameRunnerTwice() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true).Once()
	secondRunner := NewNomadJob(s.TestCtx, tests.AnotherRunnerID, nil, s.apiMock, s.nomadRunnerManager.onRunnerDestroyed)
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(secondRunner, true).Once()

	firstReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	secondReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	s.NotEqual(firstReceivedRunner, secondReceivedRunner)

	err = secondRunner.Destroy(nil)
	s.Require().NoError(err)
}

func (s *ManagerTestSuite) TestClaimAddsRunnerToUsedRunners() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	savedRunner, ok := s.nomadRunnerManager.usedRunners.Get(receivedRunner.ID())
	s.True(ok)
	s.Equal(savedRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestClaimRemovesRunnerWhenMarkAsUsedFails() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true)
	s.exerciseEnvironment.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil, false)
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	util.MaxConnectionRetriesExponential = 1
	modifyMockedCall(s.apiMock, "SetRunnerMetaUsed", func(call *mock.Call) {
		call.Run(func(_ mock.Arguments) {
			call.ReturnArguments = mock.Arguments{tests.ErrDefault}
		})
	})

	claimedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	<-time.After(time.Second + tests.ShortTimeout) // Claimed runners are marked as used asynchronously
	s.apiMock.AssertCalled(s.T(), "DeleteJob", claimedRunner.ID())
	_, ok := s.nomadRunnerManager.usedRunners.Get(claimedRunner.ID())
	s.False(ok)
}

func (s *ManagerTestSuite) TestGetReturnsRunnerIfRunnerIsUsed() {
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner.ID(), s.exerciseRunner)
	savedRunner, err := s.nomadRunnerManager.Get(s.exerciseRunner.ID())
	s.Require().NoError(err)
	s.Equal(savedRunner, s.exerciseRunner)
}

func (s *ManagerTestSuite) TestGetReturnsErrorIfRunnerNotFound() {
	savedRunner, err := s.nomadRunnerManager.Get(tests.DefaultRunnerID)
	s.Nil(savedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestReturnRemovesRunnerFromUsedRunners() {
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.exerciseEnvironment.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil, false)
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner.ID(), s.exerciseRunner)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Require().NoError(err)
	_, ok := s.nomadRunnerManager.usedRunners.Get(s.exerciseRunner.ID())
	s.False(ok)
}

func (s *ManagerTestSuite) TestReturnCallsDeleteRunnerApiMethod() {
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.exerciseEnvironment.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil, false)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Require().NoError(err)
	s.apiMock.AssertCalled(s.T(), "DeleteJob", s.exerciseRunner.ID())
}

func (s *ManagerTestSuite) TestReturnReturnsErrorWhenApiCallFailed() {
	tests.RemoveMethodFromMock(&s.apiMock.Mock, "DeleteJob")
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(tests.ErrDefault)
	defer s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	defer tests.RemoveMethodFromMock(&s.apiMock.Mock, "DeleteJob")
	s.exerciseEnvironment.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil, false)

	util.MaxConnectionRetriesExponential = 1
	util.InitialWaitingDuration = 2 * tests.ShortTimeout

	chReturnDone := make(chan error)
	go func(done chan<- error) {
		err := s.nomadRunnerManager.Return(s.exerciseRunner)
		select {
		case <-s.TestCtx.Done():
		case done <- err:
		}
		close(done)
	}(chReturnDone)

	select {
	case <-chReturnDone:
		s.Fail("Return should not return if the API request failed")
	case <-time.After(tests.ShortTimeout):
	}

	select {
	case err := <-chReturnDone:
		s.Require().ErrorIs(err, tests.ErrDefault)
	case <-time.After(2 * tests.ShortTimeout):
		s.Fail("Return should return after the retry mechanism")
		// note: MaxConnectionRetriesExponential and InitialWaitingDuration is decreased extremely here.
	}
}

func (s *ManagerTestSuite) TestUpdateRunnersLogsErrorFromWatchAllocation() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "runner")
	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(_ mock.Arguments) {
			call.ReturnArguments = mock.Arguments{tests.ErrDefault}
		})
	})

	err := s.nomadRunnerManager.SynchronizeRunners(s.TestCtx)
	if err != nil {
		log.WithError(err).Error("failed to synchronize runners")
	}

	s.Require().Len(hook.Entries, 2)
	s.Equal(logrus.ErrorLevel, hook.LastEntry().Level)
	err, ok := hook.LastEntry().Data[logrus.ErrorKey].(error)
	s.Require().True(ok)
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *ManagerTestSuite) TestUpdateRunnersAddsIdleRunner() {
	allocation := &nomadApi.Allocation{ID: tests.DefaultRunnerID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID.ToString())
	s.Require().True(ok)
	allocation.JobID = environment.ID().ToString()
	environmentMock, ok := environment.(*ExecutionEnvironmentMock)
	s.Require().True(ok)
	mockIdleRunners(environmentMock)

	_, ok = environment.Sample()
	s.Require().False(ok)

	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			callbacks, ok := args.Get(1).(*nomad.AllocationProcessing)
			s.Require().True(ok)
			callbacks.OnNew(s.TestCtx, allocation, 0)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	go func() {
		err := s.nomadRunnerManager.SynchronizeRunners(s.TestCtx)
		if err != nil {
			log.WithError(err).Error("failed to synchronize runners")
		}
	}()
	<-time.After(10 * time.Millisecond)

	r, ok := environment.Sample()
	s.True(ok)
	s.Require().NoError(r.Destroy(nil))
}

func (s *ManagerTestSuite) TestUpdateRunnersRemovesIdleAndUsedRunner() {
	allocation := &nomadApi.Allocation{JobID: tests.DefaultRunnerID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID.ToString())
	s.Require().True(ok)
	environmentMock, ok := environment.(*ExecutionEnvironmentMock)
	s.Require().True(ok)
	mockIdleRunners(environmentMock)

	testRunner := NewNomadJob(s.TestCtx, allocation.JobID, nil, s.apiMock, s.nomadRunnerManager.onRunnerDestroyed)
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	environment.AddRunner(testRunner)
	s.nomadRunnerManager.usedRunners.Add(testRunner.ID(), testRunner)

	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			callbacks, ok := args.Get(1).(*nomad.AllocationProcessing)
			s.Require().True(ok)
			callbacks.OnDeleted(s.TestCtx, allocation.JobID, nil)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	go func() {
		err := s.nomadRunnerManager.SynchronizeRunners(s.TestCtx)
		if err != nil {
			log.WithError(err).Error("failed to synchronize runners")
		}
	}()
	<-time.After(tests.ShortTimeout)

	_, ok = environment.Sample()
	s.False(ok)
	_, ok = s.nomadRunnerManager.usedRunners.Get(allocation.JobID)
	s.False(ok)
}

func modifyMockedCall(apiMock *nomad.ExecutorAPIMock, method string, modifier func(call *mock.Call)) {
	for _, c := range apiMock.ExpectedCalls {
		if c.Method == method {
			modifier(c)
		}
	}
}

func (s *ManagerTestSuite) TestOnAllocationAdded() {
	s.Run("does not add environment template id job", func() {
		environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
		s.True(ok)
		environmentMock, ok := environment.(*ExecutionEnvironmentMock)
		s.Require().True(ok)
		mockIdleRunners(environmentMock)

		alloc := &nomadApi.Allocation{JobID: nomad.TemplateJobID(tests.DefaultEnvironmentIDAsInteger)}
		s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)

		_, ok = environment.Sample()
		s.False(ok)
	})
	s.Run("does not panic when environment id cannot be parsed", func() {
		alloc := &nomadApi.Allocation{JobID: ""}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)
		})
	})
	s.Run("does not panic when environment does not exist", func() {
		nonExistentEnvironment := dto.EnvironmentID(1234)
		_, ok := s.nomadRunnerManager.environments.Get(nonExistentEnvironment.ToString())
		s.Require().False(ok)

		alloc := &nomadApi.Allocation{JobID: nomad.RunnerJobID(nonExistentEnvironment, "1-1-1-1")}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)
		})
	})
	s.Run("adds correct job", func() {
		s.Run("without allocated resources", func() {
			environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
			s.True(ok)
			environmentMock, ok := environment.(*ExecutionEnvironmentMock)
			s.Require().True(ok)
			mockIdleRunners(environmentMock)

			_, ok = environment.Sample()
			s.Require().False(ok)

			alloc := &nomadApi.Allocation{
				JobID:              tests.DefaultRunnerID,
				AllocatedResources: nil,
			}
			s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)

			runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
			s.Require().NoError(err)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(tests.DefaultRunnerID, nomadJob.id)
			s.Empty(nomadJob.portMappings)

			s.Run("but not again", func() {
				s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)
				runner, err = s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
				s.Error(err)
			})

			err = nomadJob.Destroy(nil)
			s.Require().NoError(err)
		})
		s.nomadRunnerManager.usedRunners.Purge()
		s.Run("with mapped ports", func() {
			environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
			s.True(ok)
			environmentMock, ok := environment.(*ExecutionEnvironmentMock)
			s.Require().True(ok)
			mockIdleRunners(environmentMock)

			alloc := &nomadApi.Allocation{
				JobID: tests.DefaultRunnerID,
				AllocatedResources: &nomadApi.AllocatedResources{
					Shared: nomadApi.AllocatedSharedResources{Ports: tests.DefaultPortMappings},
				},
			}
			s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)

			runner, ok := environment.Sample()
			s.True(ok)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(tests.DefaultRunnerID, nomadJob.id)
			s.Equal(nomadJob.portMappings, tests.DefaultPortMappings)

			err := runner.Destroy(nil)
			s.Require().NoError(err)
		})
	})
	s.nomadRunnerManager.usedRunners.Purge()
	s.Run("resets meta used when added allocation has a previous allocation", func() {
		environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
		s.True(ok)
		environmentMock, ok := environment.(*ExecutionEnvironmentMock)
		s.Require().True(ok)
		mockIdleRunners(environmentMock)

		alloc := &nomadApi.Allocation{JobID: tests.DefaultRunnerID, PreviousAllocation: tests.DefaultUUID}
		s.nomadRunnerManager.onAllocationAdded(s.TestCtx, alloc, 0)

		<-time.After(tests.ShortTimeout)
		s.apiMock.AssertCalled(s.T(), "SetRunnerMetaUsed", tests.DefaultRunnerID, false, 0)

		runner, ok := environment.Sample()
		s.True(ok)
		err := runner.Destroy(nil)
		s.Require().NoError(err)
	})
}

func (s *ManagerTestSuite) TestOnAllocationStopped() {
	s.Run("returns false for idle runner", func() {
		environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
		s.Require().True(ok)
		environmentMock, ok := environment.(*ExecutionEnvironmentMock)
		s.Require().True(ok)
		mockIdleRunners(environmentMock)

		r := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, []nomadApi.PortMapping{}, s.apiMock, func(_ Runner) error { return nil })
		environment.AddRunner(r)
		alreadyRemoved := s.nomadRunnerManager.onAllocationStopped(s.TestCtx, tests.DefaultRunnerID, nil)
		s.False(alreadyRemoved)
		s.Error(r.ctx.Err(), "The runner should be destroyed and its context canceled")
	})
	s.Run("returns false and stops inactivity timer", func() {
		runner, runnerDestroyed := testStoppedInactivityTimer(s)

		alreadyRemoved := s.nomadRunnerManager.onAllocationStopped(s.TestCtx, runner.ID(), nil)
		s.False(alreadyRemoved)

		select {
		case <-time.After(time.Second + tests.ShortTimeout):
			s.Fail("runner was stopped too late")
		case <-runnerDestroyed:
			s.False(runner.TimeoutPassed())
		}
	})
	s.Run("stops inactivity timer - counter check", func() {
		runner, runnerDestroyed := testStoppedInactivityTimer(s)

		select {
		case <-time.After(time.Second + tests.ShortTimeout):
			s.Fail("runner was stopped too late")
		case <-runnerDestroyed:
			s.True(runner.TimeoutPassed())
		}
	})
	s.Run("returns true when the runner is already removed", func() {
		s.Run("by the inactivity timer", func() {
			runner, _ := testStoppedInactivityTimer(s)

			<-time.After(time.Second)
			s.Require().True(runner.TimeoutPassed())

			alreadyRemoved := s.nomadRunnerManager.onAllocationStopped(s.TestCtx, runner.ID(), nil)
			s.True(alreadyRemoved)
		})
	})
	s.Run("does not log expectedly stopped environments", func() {
		logger, hook := test.NewNullLogger()
		log = logger.WithField("package", "runner")

		s.nomadRunnerManager.DeleteEnvironment(tests.DefaultEnvironmentIDAsInteger)
		alreadyRemoved := s.nomadRunnerManager.onAllocationStopped(s.TestCtx, tests.DefaultTemplateJobID, nil)
		s.True(alreadyRemoved)
		s.Empty(hook.Entries)
	})
}

func testStoppedInactivityTimer(s *ManagerTestSuite) (r Runner, destroyed chan struct{}) {
	s.T().Helper()
	environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
	s.Require().True(ok)
	environmentMock, ok := environment.(*ExecutionEnvironmentMock)
	s.Require().True(ok)
	mockIdleRunners(environmentMock)

	runnerDestroyed := make(chan struct{})
	environment.AddRunner(NewNomadJob(s.TestCtx, tests.DefaultRunnerID, []nomadApi.PortMapping{}, s.apiMock, func(runner Runner) error {
		go func() {
			select {
			case runnerDestroyed <- struct{}{}:
			case <-s.TestCtx.Done():
			}
		}()
		return s.nomadRunnerManager.onRunnerDestroyed(runner)
	}))

	runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, 1)
	s.Require().NoError(err)
	s.Require().False(runner.TimeoutPassed())
	select {
	case runnerDestroyed <- struct{}{}:
		s.Fail("The runner should not be removed by now")
	case <-time.After(tests.ShortTimeout):
	}

	return runner, runnerDestroyed
}

func (s *MainTestSuite) TestNomadRunnerManager_Load() {
	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(s.TestCtx, apiMock)
	apiMock.On("LoadRunnerPortMappings", mock.AnythingOfType("string")).
		Return([]nomadApi.PortMapping{}, nil)
	call := apiMock.On("LoadRunnerJobs", dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	runnerManager := NewNomadRunnerManager(s.TestCtx, apiMock)
	environmentMock := createBasicEnvironmentMock(tests.DefaultEnvironmentIDAsInteger)
	environmentMock.On("ApplyPrewarmingPoolSize").Return(nil)
	environmentMock.On("DeleteRunner", mock.Anything).Return(nil, true).Maybe()
	runnerManager.StoreEnvironment(environmentMock)

	s.Run("Stores unused runner", func() {
		tests.RemoveMethodFromMock(&environmentMock.Mock, "DeleteRunner")
		environmentMock.On("AddRunner", mock.AnythingOfType("*runner.NomadJob")).Once()

		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		s.ExpectedGoroutineIncrease++ // We dont care about destroying the created runner.
		call.Return([]*nomadApi.Job{job}, nil)

		runnerManager.Load(s.TestCtx)
		environmentMock.AssertExpectations(s.T())
	})

	s.Run("Stores used runner", func() {
		apiMock.On("SetRunnerMetaUsed", mock.AnythingOfType("string"), mock.AnythingOfType("bool"), mock.AnythingOfType("int")).Return(nil)
		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		s.Require().NotNil(configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		s.ExpectedGoroutineIncrease++ // We don't care about destroying the created runner.
		call.Return([]*nomadApi.Job{job}, nil)

		s.Require().Zero(runnerManager.usedRunners.Length())
		runnerManager.Load(s.TestCtx)
		_, ok := runnerManager.usedRunners.Get(tests.DefaultRunnerID)
		s.True(ok)
	})

	runnerManager.usedRunners.Purge()
	s.Run("Restart timeout of used runner", func() {
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		environmentMock.On("DeleteRunner", mock.AnythingOfType("string")).Once().Return(nil, false)
		timeout := 1

		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		s.Require().NotNil(configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey] = strconv.Itoa(timeout)
		call.Return([]*nomadApi.Job{job}, nil)

		s.Require().Zero(runnerManager.usedRunners.Length())
		runnerManager.Load(s.TestCtx)
		s.Require().NotZero(runnerManager.usedRunners.Length())

		<-time.After(time.Duration(timeout*2) * time.Second)
		s.Require().Zero(runnerManager.usedRunners.Length())
	})

	s.Run("Don't stop running executions", func() {
		apiMock.On("SetRunnerMetaUsed", mock.AnythingOfType("string"), mock.AnythingOfType("bool"), mock.AnythingOfType("int")).Return(nil).Once()
		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		s.Require().NotNil(configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey] = strconv.Itoa(1)
		call.Return([]*nomadApi.Job{job}, nil)

		executionCtx, cancelExecution := context.WithCancel(s.TestCtx)
		apiMock.On("ExecuteCommand", mock.Anything, tests.DefaultRunnerID,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ mock.Arguments) {
				<-executionCtx.Done()
			}).Return(0, nil)
		r := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error {
			return nil
		})
		runnerManager.usedRunners.Add(r.ID(), r)
		r.StoreExecution(tests.DefaultExecutionID, &dto.ExecutionRequest{})
		exitInfo, _, err := r.ExecuteInteractively(s.TestCtx, tests.DefaultExecutionID,
			&nullio.ReadWriter{Reader: nullio.Reader{Ctx: s.TestCtx}}, io.Discard, io.Discard)
		s.Require().NoError(err)

		runnerManager.Load(s.TestCtx)
		select {
		case <-exitInfo:
			s.FailNow("Execution stopped on recovery")
		case <-time.After(tests.ShortTimeout):
		}

		cancelExecution()
		<-time.After(tests.ShortTimeout)
		err = r.Destroy(ErrLocalDestruction)
		s.Require().NoError(err)
	})

	s.Run("Update the Port Mapping", func() {
		updatedPortMapping := nomadApi.PortMapping{
			Label:  "ssh",
			Value:  10022,
			To:     22,
			HostIP: "127.0.0.1",
		}
		updatedMappedPort := &dto.MappedPort{
			ExposedPort: 22,
			HostAddress: "127.0.0.1:10022",
		}

		tests.RemoveMethodFromMock(&apiMock.Mock, "LoadRunnerPortMappings")
		apiMock.On("LoadRunnerPortMappings", mock.Anything).
			Return([]nomadApi.PortMapping{updatedPortMapping}, nil).Once()

		apiMock.On("SetRunnerMetaUsed", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		s.Require().NotNil(configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey] = strconv.Itoa(1)
		call.Return([]*nomadApi.Job{job}, nil)
		r := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error {
			return nil
		})
		runnerManager.usedRunners.Add(r.ID(), r)

		s.Empty(r.MappedPorts())
		runnerManager.Load(s.TestCtx)
		s.Require().Len(r.MappedPorts(), 1)
		s.Equal(updatedMappedPort, r.MappedPorts()[0])

		err := r.Destroy(ErrLocalDestruction)
		s.Require().NoError(err)
	})
}

func (s *MainTestSuite) TestNomadRunnerManager_checkPrewarmingPoolAlert() {
	const timeout = 1
	config.Config.Server.Alert.PrewarmingPoolReloadTimeout = uint(timeout)
	config.Config.Server.Alert.PrewarmingPoolThreshold = 0.5
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	environment.On("Image").Return("")
	environment.On("CPULimit").Return(uint(0))
	environment.On("MemoryLimit").Return(uint(0))
	environment.On("NetworkAccess").Return(false, nil)
	apiMock := &nomad.ExecutorAPIMock{}
	runnerManager := NewNomadRunnerManager(s.TestCtx, apiMock)
	runnerManager.StoreEnvironment(environment)
	s.Run("checks the alert condition again after the reload timeout", func() {
		environment.On("PrewarmingPoolSize").Return(uint(1)).Once()
		environment.On("IdleRunnerCount").Return(uint(0)).Once()
		environment.On("PrewarmingPoolSize").Return(uint(1)).Once()
		environment.On("IdleRunnerCount").Return(uint(1)).Once()

		checkDone := make(chan struct{})
		go func() {
			runnerManager.checkPrewarmingPoolAlert(s.TestCtx, environment, false)
			close(checkDone)
		}()

		select {
		case <-checkDone:
			s.Fail("checkPrewarmingPoolAlert returned before the reload timeout")
		case <-time.After(time.Duration(timeout) * time.Second / 2):
		}

		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			s.Fail("checkPrewarmingPoolAlert did not return after checking the alert condition again")
		case <-checkDone:
		}
		environment.AssertExpectations(s.T())
	})
	s.Run("checks the alert condition again after the reload timeout", func() {
		environment.On("PrewarmingPoolSize").Return(uint(1)).Twice()
		environment.On("IdleRunnerCount").Return(uint(0)).Twice()
		apiMock.On("LoadRunnerJobs", environment.ID()).Return([]*nomadApi.Job{}, nil).Once()
		environment.On("ApplyPrewarmingPoolSize").Return(nil).Once()

		checkDone := make(chan struct{})
		go func() {
			runnerManager.checkPrewarmingPoolAlert(s.TestCtx, environment, false)
			close(checkDone)
		}()

		select {
		case <-time.After(time.Duration(timeout) * time.Second * 2):
			s.Fail("checkPrewarmingPoolAlert did not return")
		case <-checkDone:
		}
		environment.AssertExpectations(s.T())
	})
	s.Run("is canceled by an added runner", func() {
		environment.On("PrewarmingPoolSize").Return(uint(1)).Twice()
		environment.On("IdleRunnerCount").Return(uint(0)).Once()
		environment.On("IdleRunnerCount").Return(uint(1)).Once()

		checkDone := make(chan struct{})
		go func() {
			runnerManager.checkPrewarmingPoolAlert(s.TestCtx, environment, false)
			close(checkDone)
		}()

		<-time.After(tests.ShortTimeout)
		go runnerManager.checkPrewarmingPoolAlert(s.TestCtx, environment, true)
		<-time.After(tests.ShortTimeout)

		select {
		case <-time.After(100 * time.Duration(timeout) * time.Second):
			s.Fail("checkPrewarmingPoolAlert was not canceled")
		case <-checkDone:
		}
		environment.AssertExpectations(s.T())
	})
}

func (s *MainTestSuite) TestNomadRunnerManager_checkPrewarmingPoolAlert_reloadsRunners() {
	config.Config.Server.Alert.PrewarmingPoolReloadTimeout = uint(1)
	config.Config.Server.Alert.PrewarmingPoolThreshold = 0.5
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	environment.On("Image").Return("")
	environment.On("CPULimit").Return(uint(0))
	environment.On("MemoryLimit").Return(uint(0))
	environment.On("NetworkAccess").Return(false, nil)
	apiMock := &nomad.ExecutorAPIMock{}
	runnerManager := NewNomadRunnerManager(s.TestCtx, apiMock)
	runnerManager.StoreEnvironment(environment)

	environment.On("PrewarmingPoolSize").Return(uint(1)).Twice()
	environment.On("IdleRunnerCount").Return(uint(0)).Twice()
	environment.On("DeleteRunner", mock.Anything).Return(nil, false).Once()

	s.Require().Empty(runnerManager.usedRunners.Length())
	_, usedJob := helpers.CreateTemplateJob()
	id := tests.DefaultRunnerID
	usedJob.ID = &id
	configTaskGroup := nomad.FindTaskGroup(usedJob, nomad.ConfigTaskGroupName)
	configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
	configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey] = "42"
	_, idleJob := helpers.CreateTemplateJob()
	idleID := tests.AnotherRunnerID
	idleJob.ID = &idleID
	nomad.FindTaskGroup(idleJob, nomad.ConfigTaskGroupName).Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUnusedValue
	apiMock.On("LoadRunnerJobs", environment.ID()).Return([]*nomadApi.Job{usedJob, idleJob}, nil).Once()
	apiMock.On("LoadRunnerPortMappings", mock.Anything).Return(nil, nil).Twice()
	environment.On("ApplyPrewarmingPoolSize").Return(nil).Once()
	environment.On("AddRunner", mock.Anything).Run(func(args mock.Arguments) {
		job, ok := args[0].(*NomadJob)
		s.Require().True(ok)
		err := job.Destroy(ErrLocalDestruction)
		s.Require().NoError(err)
	}).Return().Once()

	runnerManager.checkPrewarmingPoolAlert(s.TestCtx, environment, false)

	r, ok := runnerManager.usedRunners.Get(tests.DefaultRunnerID)
	s.Require().True(ok)
	err := r.Destroy(ErrLocalDestruction)
	s.Require().NoError(err)

	environment.AssertExpectations(s.T())
}

func mockWatchAllocations(ctx context.Context, apiMock *nomad.ExecutorAPIMock) {
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(_ mock.Arguments) {
		<-ctx.Done()
		call.ReturnArguments = mock.Arguments{nil}
	})
}
