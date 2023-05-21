package runner

import (
	"context"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"strconv"
	"testing"
	"time"
)

func TestGetNextRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}

type ManagerTestSuite struct {
	suite.Suite
	apiMock             *nomad.ExecutorAPIMock
	nomadRunnerManager  *NomadRunnerManager
	exerciseEnvironment *ExecutionEnvironmentMock
	exerciseRunner      Runner
}

func (s *ManagerTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorAPIMock{}
	mockRunnerQueries(s.apiMock, []string{})
	// Instantly closed context to manually start the update process in some cases
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.nomadRunnerManager = NewNomadRunnerManager(s.apiMock, ctx)

	s.exerciseRunner = NewRunner(tests.DefaultRunnerID, s.nomadRunnerManager)
	s.exerciseEnvironment = createBasicEnvironmentMock(defaultEnvironmentID)
	s.nomadRunnerManager.StoreEnvironment(s.exerciseEnvironment)
}

func mockRunnerQueries(apiMock *nomad.ExecutorAPIMock, returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	apiMock.ExpectedCalls = []*mock.Call{}
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-time.After(tests.DefaultTestTimeout)
		call.ReturnArguments = mock.Arguments{nil}
	})
	apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
	apiMock.On("MarkRunnerAsUsed", mock.AnythingOfType("string"), mock.AnythingOfType("int")).Return(nil)
	apiMock.On("LoadRunnerIDs", tests.DefaultRunnerID).Return(returnedRunnerIds, nil)
	apiMock.On("JobScale", tests.DefaultRunnerID).Return(uint(len(returnedRunnerIds)), nil)
	apiMock.On("SetJobScale", tests.DefaultRunnerID, mock.AnythingOfType("uint"), "Runner Requested").Return(nil)
	apiMock.On("RegisterRunnerJob", mock.Anything).Return(nil)
	apiMock.On("MonitorEvaluation", mock.Anything, mock.Anything).Return(nil)
}

func mockIdleRunners(environmentMock *ExecutionEnvironmentMock) {
	idleRunner := storage.NewLocalStorage[Runner]()
	environmentMock.On("AddRunner", mock.Anything).Run(func(args mock.Arguments) {
		r, ok := args.Get(0).(Runner)
		if !ok {
			return
		}
		idleRunner.Add(r.ID(), r)
	})
	sampleCall := environmentMock.On("Sample", mock.Anything)
	sampleCall.Run(func(args mock.Arguments) {
		r, ok := idleRunner.Sample()
		sampleCall.ReturnArguments = mock.Arguments{r, ok}
	})
	deleteCall := environmentMock.On("DeleteRunner", mock.AnythingOfType("string"))
	deleteCall.Run(func(args mock.Arguments) {
		id, ok := args.Get(0).(string)
		if !ok {
			return
		}
		idleRunner.Delete(id)
	})
}

func (s *ManagerTestSuite) waitForRunnerRefresh() {
	<-time.After(100 * time.Millisecond)
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
	s.Equal(ErrUnknownExecutionEnvironment, err)
}

func (s *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	s.exerciseEnvironment.On("Sample", mock.Anything).Return(s.exerciseRunner, true)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
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
	s.exerciseEnvironment.On("Sample", mock.Anything).
		Return(NewRunner(tests.AnotherRunnerID, s.nomadRunnerManager), true).Once()

	firstReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
	secondReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
	s.NotEqual(firstReceivedRunner, secondReceivedRunner)
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
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	util.MaxConnectionRetriesExponential = 1
	modifyMockedCall(s.apiMock, "MarkRunnerAsUsed", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
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
	s.NoError(err)
	s.Equal(savedRunner, s.exerciseRunner)
}

func (s *ManagerTestSuite) TestGetReturnsErrorIfRunnerNotFound() {
	savedRunner, err := s.nomadRunnerManager.Get(tests.DefaultRunnerID)
	s.Nil(savedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestReturnRemovesRunnerFromUsedRunners() {
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner.ID(), s.exerciseRunner)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Nil(err)
	_, ok := s.nomadRunnerManager.usedRunners.Get(s.exerciseRunner.ID())
	s.False(ok)
}

func (s *ManagerTestSuite) TestReturnCallsDeleteRunnerApiMethod() {
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Nil(err)
	s.apiMock.AssertCalled(s.T(), "DeleteJob", s.exerciseRunner.ID())
}

func (s *ManagerTestSuite) TestReturnReturnsErrorWhenApiCallFailed() {
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(tests.ErrDefault)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestUpdateRunnersLogsErrorFromWatchAllocation() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "runner")
	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{tests.ErrDefault}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

	s.Require().Equal(1, len(hook.Entries))
	s.Equal(logrus.ErrorLevel, hook.LastEntry().Level)
	s.Equal(hook.LastEntry().Data[logrus.ErrorKey], tests.ErrDefault)
}

func (s *ManagerTestSuite) TestUpdateRunnersAddsIdleRunner() {
	allocation := &nomadApi.Allocation{ID: tests.DefaultRunnerID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID.ToString())
	s.Require().True(ok)
	allocation.JobID = environment.ID().ToString()
	mockIdleRunners(environment.(*ExecutionEnvironmentMock))

	_, ok = environment.Sample()
	s.Require().False(ok)

	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			callbacks, ok := args.Get(1).(*nomad.AllocationProcessing)
			s.Require().True(ok)
			callbacks.OnNew(allocation, 0)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

	_, ok = environment.Sample()
	s.True(ok)
}

func (s *ManagerTestSuite) TestUpdateRunnersRemovesIdleAndUsedRunner() {
	allocation := &nomadApi.Allocation{JobID: tests.DefaultRunnerID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID.ToString())
	s.Require().True(ok)
	mockIdleRunners(environment.(*ExecutionEnvironmentMock))

	testRunner := NewRunner(allocation.JobID, s.nomadRunnerManager)
	environment.AddRunner(testRunner)
	s.nomadRunnerManager.usedRunners.Add(testRunner.ID(), testRunner)

	modifyMockedCall(s.apiMock, "WatchEventStream", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			callbacks, ok := args.Get(1).(*nomad.AllocationProcessing)
			s.Require().True(ok)
			callbacks.OnDeleted(allocation, false)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

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
		mockIdleRunners(environment.(*ExecutionEnvironmentMock))

		alloc := &nomadApi.Allocation{JobID: nomad.TemplateJobID(tests.DefaultEnvironmentIDAsInteger)}
		s.nomadRunnerManager.onAllocationAdded(alloc, 0)

		_, ok = environment.Sample()
		s.False(ok)
	})
	s.Run("does not panic when environment id cannot be parsed", func() {
		alloc := &nomadApi.Allocation{JobID: ""}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(alloc, 0)
		})
	})
	s.Run("does not panic when environment does not exist", func() {
		nonExistentEnvironment := dto.EnvironmentID(1234)
		_, ok := s.nomadRunnerManager.environments.Get(nonExistentEnvironment.ToString())
		s.Require().False(ok)

		alloc := &nomadApi.Allocation{JobID: nomad.RunnerJobID(nonExistentEnvironment, "1-1-1-1")}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(alloc, 0)
		})
	})
	s.Run("adds correct job", func() {
		s.Run("without allocated resources", func() {
			environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
			s.True(ok)
			mockIdleRunners(environment.(*ExecutionEnvironmentMock))

			_, ok = environment.Sample()
			s.Require().False(ok)

			alloc := &nomadApi.Allocation{
				JobID:              tests.DefaultRunnerID,
				AllocatedResources: nil,
			}
			s.nomadRunnerManager.onAllocationAdded(alloc, 0)

			runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
			s.NoError(err)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(nomadJob.id, tests.DefaultRunnerID)
			s.Empty(nomadJob.portMappings)

			s.Run("but not again", func() {
				s.nomadRunnerManager.onAllocationAdded(alloc, 0)
				runner, err = s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
				s.Error(err)
			})
		})
		s.nomadRunnerManager.usedRunners.Purge()
		s.Run("with mapped ports", func() {
			environment, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsString)
			s.True(ok)
			mockIdleRunners(environment.(*ExecutionEnvironmentMock))

			alloc := &nomadApi.Allocation{
				JobID: tests.DefaultRunnerID,
				AllocatedResources: &nomadApi.AllocatedResources{
					Shared: nomadApi.AllocatedSharedResources{Ports: tests.DefaultPortMappings},
				},
			}
			s.nomadRunnerManager.onAllocationAdded(alloc, 0)

			runner, ok := environment.Sample()
			s.True(ok)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(nomadJob.id, tests.DefaultRunnerID)
			s.Equal(nomadJob.portMappings, tests.DefaultPortMappings)
		})
	})
}

func TestNomadRunnerManager_Load(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	mockWatchAllocations(apiMock)
	apiMock.On("LoadRunnerPortMappings", mock.AnythingOfType("string")).
		Return([]nomadApi.PortMapping{}, nil)
	call := apiMock.On("LoadRunnerJobs", dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	runnerManager := NewNomadRunnerManager(apiMock, context.Background())
	environmentMock := createBasicEnvironmentMock(tests.DefaultEnvironmentIDAsInteger)
	environmentMock.On("ApplyPrewarmingPoolSize").Return(nil)
	runnerManager.StoreEnvironment(environmentMock)

	t.Run("Stores unused runner", func(t *testing.T) {
		environmentMock.On("AddRunner", mock.AnythingOfType("*runner.NomadJob")).Once()

		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		call.Return([]*nomadApi.Job{job}, nil)

		runnerManager.Load()

		environmentMock.AssertExpectations(t)
	})

	t.Run("Stores used runner", func(t *testing.T) {
		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		require.NotNil(t, configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		call.Return([]*nomadApi.Job{job}, nil)

		require.Zero(t, runnerManager.usedRunners.Length())

		runnerManager.Load()

		_, ok := runnerManager.usedRunners.Get(tests.DefaultRunnerID)
		assert.True(t, ok)
	})

	runnerManager.usedRunners.Purge()
	t.Run("Restart timeout of used runner", func(t *testing.T) {
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		timeout := 1

		_, job := helpers.CreateTemplateJob()
		jobID := tests.DefaultRunnerID
		job.ID = &jobID
		job.Name = &jobID
		configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
		require.NotNil(t, configTaskGroup)
		configTaskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUsedValue
		configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey] = strconv.Itoa(timeout)
		call.Return([]*nomadApi.Job{job}, nil)

		require.Zero(t, runnerManager.usedRunners.Length())

		runnerManager.Load()

		require.NotZero(t, runnerManager.usedRunners.Length())

		<-time.After(time.Duration(timeout*2) * time.Second)
		require.Zero(t, runnerManager.usedRunners.Length())
	})
}

func mockWatchAllocations(apiMock *nomad.ExecutorAPIMock) {
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-time.After(tests.DefaultTestTimeout)
		call.ReturnArguments = mock.Arguments{nil}
	})
}
