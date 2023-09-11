package nomad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	noopAllocationProcessing = &AllocationProcessing{
		OnNew:     func(_ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ string, _ error) bool { return false },
	}
	ErrUnexpectedEOF = errors.New("unexpected EOF")
)

func TestLoadRunnersTestSuite(t *testing.T) {
	suite.Run(t, new(LoadRunnersTestSuite))
}

type LoadRunnersTestSuite struct {
	tests.MemoryLeakTestSuite
	jobID                  string
	mock                   *apiQuerierMock
	nomadAPIClient         APIClient
	availableRunner        *nomadApi.JobListStub
	anotherAvailableRunner *nomadApi.JobListStub
	pendingRunner          *nomadApi.JobListStub
	deadRunner             *nomadApi.JobListStub
}

func (s *LoadRunnersTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.jobID = tests.DefaultRunnerID

	s.mock = &apiQuerierMock{}
	s.nomadAPIClient = APIClient{apiQuerier: s.mock}

	s.availableRunner = newJobListStub(tests.DefaultRunnerID, structs.JobStatusRunning, 1)
	s.anotherAvailableRunner = newJobListStub(tests.AnotherRunnerID, structs.JobStatusRunning, 1)
	s.pendingRunner = newJobListStub(tests.DefaultRunnerID+"-1", structs.JobStatusPending, 0)
	s.deadRunner = newJobListStub(tests.AnotherRunnerID+"-1", structs.JobStatusDead, 0)
}

func newJobListStub(id, status string, amountRunning int) *nomadApi.JobListStub {
	return &nomadApi.JobListStub{
		ID:     id,
		Status: status,
		JobSummary: &nomadApi.JobSummary{
			JobID:   id,
			Summary: map[string]nomadApi.TaskGroupSummary{TaskGroupName: {Running: amountRunning}},
		},
	}
}

func (s *LoadRunnersTestSuite) TestErrorOfUnderlyingApiCallIsPropagated() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return(nil, tests.ErrDefault)

	returnedIds, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Nil(returnedIds)
	s.Equal(tests.ErrDefault, err)
}

func (s *LoadRunnersTestSuite) TestReturnsNoErrorWhenUnderlyingApiCallDoesNot() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{}, nil)

	_, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.NoError(err)
}

func (s *LoadRunnersTestSuite) TestAvailableRunnerIsReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.availableRunner}, nil)

	returnedIds, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIds, 1)
	s.Equal(s.availableRunner.ID, returnedIds[0])
}

func (s *LoadRunnersTestSuite) TestPendingRunnerIsReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.pendingRunner}, nil)

	returnedIds, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIds, 1)
	s.Equal(s.pendingRunner.ID, returnedIds[0])
}

func (s *LoadRunnersTestSuite) TestDeadRunnerIsNotReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.deadRunner}, nil)

	returnedIds, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Empty(returnedIds)
}

func (s *LoadRunnersTestSuite) TestReturnsAllAvailableRunners() {
	runnersList := []*nomadApi.JobListStub{
		s.availableRunner,
		s.anotherAvailableRunner,
		s.pendingRunner,
		s.deadRunner,
	}
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return(runnersList, nil)

	returnedIds, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIds, 3)
	s.Contains(returnedIds, s.availableRunner.ID)
	s.Contains(returnedIds, s.anotherAvailableRunner.ID)
	s.Contains(returnedIds, s.pendingRunner.ID)
	s.NotContains(returnedIds, s.deadRunner.ID)
}

const TestNamespace = "unit-tests"
const TestNomadToken = "n0m4d-t0k3n"
const TestDefaultAddress = "127.0.0.1"
const evaluationID = "evaluation-id"

func NomadTestConfig(address string) *config.Nomad {
	return &config.Nomad{
		Address: address,
		Port:    4646,
		Token:   TestNomadToken,
		TLS: config.TLS{
			Active: false,
		},
		Namespace: TestNamespace,
	}
}

func (s *MainTestSuite) TestApiClient_init() {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(NomadTestConfig(TestDefaultAddress))
	s.Require().Nil(err)
}

func (s *MainTestSuite) TestApiClientCanNotBeInitializedWithInvalidUrl() {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(NomadTestConfig("http://" + TestDefaultAddress))
	s.NotNil(err)
}

func (s *MainTestSuite) TestNewExecutorApiCanBeCreatedWithoutError() {
	expectedClient := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := expectedClient.init(NomadTestConfig(TestDefaultAddress))
	s.Require().Nil(err)

	_, err = NewExecutorAPI(NomadTestConfig(TestDefaultAddress))
	s.Require().Nil(err)
}

// asynchronouslyMonitorEvaluation creates an APIClient with mocked Nomad API and
// runs the MonitorEvaluation method in a goroutine. The mock returns a read-only
// version of the given stream to simulate an event stream gotten from the real
// Nomad API.
func asynchronouslyMonitorEvaluation(stream <-chan *nomadApi.Events) chan error {
	ctx := context.Background()
	// We can only get a read-only channel once we return it from a function.
	readOnlyStream := func() <-chan *nomadApi.Events { return stream }()

	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.AnythingOfType("*context.cancelCtx")).
		Return(readOnlyStream, nil)
	apiClient := &APIClient{apiMock, map[string]chan error{}, storage.NewLocalStorage[*allocationData](), false}

	errChan := make(chan error)
	go func() {
		errChan <- apiClient.MonitorEvaluation(evaluationID, ctx)
	}()
	return errChan
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationReturnsNilWhenStreamIsClosed() {
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyMonitorEvaluation(stream)

	close(stream)
	var err error
	// If close doesn't terminate MonitorEvaluation, this test won't complete without a timeout.
	select {
	case err = <-errChan:
	case <-time.After(time.Millisecond * 10):
		s.T().Fatal("MonitorEvaluation didn't finish as expected")
	}
	s.Nil(err)
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationReturnsErrorWhenStreamReturnsError() {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.AnythingOfType("*context.cancelCtx")).
		Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, map[string]chan error{}, storage.NewLocalStorage[*allocationData](), false}
	err := apiClient.MonitorEvaluation("id", context.Background())
	s.ErrorIs(err, tests.ErrDefault)
}

type eventPayload struct {
	Evaluation *nomadApi.Evaluation
	Allocation *nomadApi.Allocation
}

// eventForEvaluation takes an evaluation and creates an Event with the given evaluation
// as its payload. Nomad uses the mapstructure library to decode the payload, which we
// simply reverse here.
func eventForEvaluation(t *testing.T, eval *nomadApi.Evaluation) nomadApi.Event {
	t.Helper()
	payload := make(map[string]interface{})

	err := mapstructure.Decode(eventPayload{Evaluation: eval}, &payload)
	if err != nil {
		t.Fatalf("Couldn't decode evaluation %v into payload map", eval)
		return nomadApi.Event{}
	}
	event := nomadApi.Event{Topic: nomadApi.TopicEvaluation, Payload: payload}
	return event
}

// simulateNomadEventStream streams the given events sequentially to the stream channel.
// It returns how many events have been processed until an error occurred.
func simulateNomadEventStream(
	stream chan<- *nomadApi.Events,
	errChan chan error,
	events []*nomadApi.Events,
) (int, error) {
	eventsProcessed := 0
	var e *nomadApi.Events
	for _, e = range events {
		select {
		case err := <-errChan:
			return eventsProcessed, err
		case stream <- e:
			eventsProcessed++
		}
	}
	close(stream)
	// Wait for last event being processed
	var err error
	select {
	case <-time.After(10 * time.Millisecond):
	case err = <-errChan:
	}
	return eventsProcessed, err
}

// runEvaluationMonitoring simulates events streamed from the Nomad event stream
// to the MonitorEvaluation method. It starts the MonitorEvaluation function as a goroutine
// and sequentially transfers the events from the given array to a channel simulating the stream.
func runEvaluationMonitoring(events []*nomadApi.Events) (eventsProcessed int, err error) {
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyMonitorEvaluation(stream)
	return simulateNomadEventStream(stream, errChan, events)
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationWithSuccessfulEvent() {
	eval := nomadApi.Evaluation{Status: structs.EvalStatusComplete}
	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	// make sure that the tested function can complete
	s.Require().Nil(checkEvaluation(&eval))

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(s.T(), &pendingEval), eventForEvaluation(s.T(), &eval),
	}}

	var cases = []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		name                    string
	}{
		{[]*nomadApi.Events{&events}, 1,
			"it completes with successful event"},
		{[]*nomadApi.Events{&events, &events}, 2,
			"it keeps listening after first successful event"},
		{[]*nomadApi.Events{{}, &events}, 2,
			"it skips heartbeat and completes"},
		{[]*nomadApi.Events{&pendingEvaluationEvents, &events}, 2,
			"it skips pending evaluation and completes"},
		{[]*nomadApi.Events{&multipleEventsWithPending}, 1,
			"it handles multiple events per received event"},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			eventsProcessed, err := runEvaluationMonitoring(c.streamedEvents)
			s.Nil(err)
			s.Equal(c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationWithFailingEvent() {
	eval := nomadApi.Evaluation{ID: evaluationID, Status: structs.EvalStatusFailed}
	evalErr := checkEvaluation(&eval)
	s.Require().NotNil(evalErr)

	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(s.T(), &pendingEval), eventForEvaluation(s.T(), &eval),
	}}
	eventsWithErr := nomadApi.Events{Err: tests.ErrDefault, Events: []nomadApi.Event{{}}}

	var cases = []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		expectedError           error
		name                    string
	}{
		{[]*nomadApi.Events{&events}, 1, evalErr,
			"it fails with failing event"},
		{[]*nomadApi.Events{{}, &events}, 2, evalErr,
			"it skips heartbeat and fail"},
		{[]*nomadApi.Events{&pendingEvaluationEvents, &events}, 2, evalErr,
			"it skips pending evaluation and fail"},
		{[]*nomadApi.Events{&multipleEventsWithPending}, 1, evalErr,
			"it handles multiple events per received event and fails"},
		{[]*nomadApi.Events{&eventsWithErr}, 1, tests.ErrDefault,
			"it fails with event error when event has error"},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			eventsProcessed, err := runEvaluationMonitoring(c.streamedEvents)
			s.Require().NotNil(err)
			s.Contains(err.Error(), c.expectedError.Error())
			s.Equal(c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationFailsWhenFailingToDecodeEvaluation() {
	event := nomadApi.Event{
		Topic: nomadApi.TopicEvaluation,
		// This should fail decoding, as Evaluation.Status is expected to be a string, not int
		Payload: map[string]interface{}{"Evaluation": map[string]interface{}{"Status": 1}},
	}
	_, err := event.Evaluation()
	s.Require().NotNil(err)
	eventsProcessed, err := runEvaluationMonitoring([]*nomadApi.Events{{Events: []nomadApi.Event{event}}})
	s.Error(err)
	s.Equal(1, eventsProcessed)
}

func (s *MainTestSuite) TestCheckEvaluationWithFailedAllocations() {
	testKey := "test1"
	failedAllocs := map[string]*nomadApi.AllocationMetric{
		testKey: {NodesExhausted: 1},
	}
	evaluation := nomadApi.Evaluation{FailedTGAllocs: failedAllocs, Status: structs.EvalStatusFailed}

	assertMessageContainsCorrectStrings := func(msg string) {
		s.Contains(msg, evaluation.Status, "error should contain the evaluation status")
		s.Contains(msg, fmt.Sprintf("%s: %#v", testKey, failedAllocs[testKey]),
			"error should contain the failed allocations metric")
	}

	var msgWithoutBlockedEval, msgWithBlockedEval string
	s.Run("without blocked eval", func() {
		err := checkEvaluation(&evaluation)
		s.Require().NotNil(err)
		msgWithoutBlockedEval = err.Error()
		assertMessageContainsCorrectStrings(msgWithoutBlockedEval)
	})

	s.Run("with blocked eval", func() {
		evaluation.BlockedEval = "blocking-eval"
		err := checkEvaluation(&evaluation)
		s.Require().NotNil(err)
		msgWithBlockedEval = err.Error()
		assertMessageContainsCorrectStrings(msgWithBlockedEval)
	})

	s.NotEqual(msgWithBlockedEval, msgWithoutBlockedEval)
}

func (s *MainTestSuite) TestCheckEvaluationWithoutFailedAllocations() {
	evaluation := nomadApi.Evaluation{FailedTGAllocs: make(map[string]*nomadApi.AllocationMetric)}

	s.Run("when evaluation status complete", func() {
		evaluation.Status = structs.EvalStatusComplete
		err := checkEvaluation(&evaluation)
		s.Nil(err)
	})

	s.Run("when evaluation status not complete", func() {
		incompleteStates := []string{structs.EvalStatusFailed, structs.EvalStatusCancelled,
			structs.EvalStatusBlocked, structs.EvalStatusPending}
		for _, status := range incompleteStates {
			evaluation.Status = status
			err := checkEvaluation(&evaluation)
			s.Require().NotNil(err)
			s.Contains(err.Error(), status, "error should contain the evaluation status")
		}
	})
}

func (s *MainTestSuite) TestApiClient_WatchAllocationsIgnoresOldAllocations() {
	oldStoppedAllocation := createOldAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusStop)
	oldPendingAllocation := createOldAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	oldRunningAllocation := createOldAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	oldAllocationEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), oldStoppedAllocation),
		eventForAllocation(s.T(), oldPendingAllocation),
		eventForAllocation(s.T(), oldRunningAllocation),
	}}

	assertWatchAllocation(s.T(), []*nomadApi.Events{&oldAllocationEvents},
		[]*nomadApi.Allocation(nil), []string(nil))
}

func createOldAllocation(clientStatus, desiredStatus string) *nomadApi.Allocation {
	return createAllocation(time.Now().Add(-time.Minute).UnixNano(), clientStatus, desiredStatus)
}

func (s *MainTestSuite) TestApiClient_WatchAllocationsIgnoresUnhandledEvents() {
	nodeEvents := nomadApi.Events{Events: []nomadApi.Event{
		{
			Topic: nomadApi.TopicNode,
			Type:  structs.TypeNodeEvent,
		},
	}}
	assertWatchAllocation(s.T(), []*nomadApi.Events{&nodeEvents}, []*nomadApi.Allocation(nil), []string(nil))
}

func (s *MainTestSuite) TestApiClient_WatchAllocationsUsesCallbacksForEvents() {
	pendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	pendingEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), pendingAllocation)}}

	s.Run("it does not add allocation when client status is pending", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingEvents}, []*nomadApi.Allocation(nil), []string(nil))
	})

	startedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	startedEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), startedAllocation)}}
	pendingStartedEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), pendingAllocation), eventForAllocation(s.T(), startedAllocation)}}

	s.Run("it adds allocation with matching events", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})

	s.Run("it skips heartbeat and adds allocation with matching events", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})

	stoppedAllocation := createRecentAllocation(structs.AllocClientStatusComplete, structs.AllocDesiredStatusStop)
	stoppedEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), stoppedAllocation)}}
	pendingStartStopEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), pendingAllocation),
		eventForAllocation(s.T(), startedAllocation),
		eventForAllocation(s.T(), stoppedAllocation),
	}}

	s.Run("it adds and deletes the allocation", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartStopEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{stoppedAllocation.JobID})
	})

	s.Run("it ignores duplicate events", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingEvents, &startedEvents, &startedEvents,
			&stoppedEvents, &stoppedEvents, &stoppedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{startedAllocation.JobID})
	})

	s.Run("it ignores events of unknown allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&startedEvents, &startedEvents,
			&stoppedEvents, &stoppedEvents, &stoppedEvents}, []*nomadApi.Allocation(nil), []string(nil))
	})

	s.Run("it removes restarted allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents, &pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation, startedAllocation}, []string{startedAllocation.JobID})
	})

	rescheduleAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	rescheduleAllocation.ID = tests.AnotherUUID
	rescheduleAllocation.PreviousAllocation = pendingAllocation.ID
	rescheduleStartedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	rescheduleStartedAllocation.ID = tests.AnotherUUID
	rescheduleAllocation.PreviousAllocation = pendingAllocation.ID
	rescheduleEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), rescheduleAllocation), eventForAllocation(s.T(), rescheduleStartedAllocation)}}

	s.Run("it removes rescheduled allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents, &rescheduleEvents},
			[]*nomadApi.Allocation{startedAllocation, rescheduleStartedAllocation}, []string{startedAllocation.JobID})
	})

	stoppedPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusStop)
	stoppedPendingEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), stoppedPendingAllocation)}}

	s.Run("it removes stopped pending allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingEvents, &stoppedPendingEvents},
			[]*nomadApi.Allocation(nil), []string{stoppedPendingAllocation.JobID})
	})

	failedAllocation := createRecentAllocation(structs.AllocClientStatusFailed, structs.AllocDesiredStatusStop)
	failedEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), failedAllocation)}}

	s.Run("it removes stopped failed allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents, &failedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{failedAllocation.JobID})
	})

	lostAllocation := createRecentAllocation(structs.AllocClientStatusLost, structs.AllocDesiredStatusStop)
	lostEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), lostAllocation)}}

	s.Run("it removes stopped lost allocations", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents, &lostEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{lostAllocation.JobID})
	})

	rescheduledLostAllocation := createRecentAllocation(structs.AllocClientStatusLost, structs.AllocDesiredStatusStop)
	rescheduledLostAllocation.NextAllocation = tests.AnotherUUID
	rescheduledLostEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), rescheduledLostAllocation)}}

	s.Run("it removes lost allocations not before the last restart attempt", func() {
		assertWatchAllocation(s.T(), []*nomadApi.Events{&pendingStartedEvents, &rescheduledLostEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})
}

func (s *MainTestSuite) TestHandleAllocationEventBuffersPendingAllocation() {
	s.Run("AllocationUpdated", func() {
		newPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
		newPendingEvent := eventForAllocation(s.T(), newPendingAllocation)

		allocations := storage.NewLocalStorage[*allocationData]()
		err := handleAllocationEvent(
			time.Now().UnixNano(), allocations, &newPendingEvent, noopAllocationProcessing)
		s.Require().NoError(err)

		_, ok := allocations.Get(newPendingAllocation.ID)
		s.True(ok)
	})
	s.Run("PlanResult", func() {
		newPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
		newPendingEvent := eventForAllocation(s.T(), newPendingAllocation)
		newPendingEvent.Type = structs.TypePlanResult

		allocations := storage.NewLocalStorage[*allocationData]()
		err := handleAllocationEvent(
			time.Now().UnixNano(), allocations, &newPendingEvent, noopAllocationProcessing)
		s.Require().NoError(err)

		_, ok := allocations.Get(newPendingAllocation.ID)
		s.True(ok)
	})
}

func (s *MainTestSuite) TestHandleAllocationEvent_IgnoresReschedulesForStoppedJobs() {
	startedAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	rescheduledAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	rescheduledAllocation.ID = tests.AnotherUUID
	rescheduledAllocation.PreviousAllocation = startedAllocation.ID
	rescheduledEvent := eventForAllocation(s.T(), rescheduledAllocation)

	allocations := storage.NewLocalStorage[*allocationData]()
	allocations.Add(startedAllocation.ID, &allocationData{jobID: startedAllocation.JobID})

	err := handleAllocationEvent(time.Now().UnixNano(), allocations, &rescheduledEvent, &AllocationProcessing{
		OnNew:     func(_ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ string, _ error) bool { return true },
	})
	s.Require().NoError(err)

	_, ok := allocations.Get(rescheduledAllocation.ID)
	s.False(ok)
}

func (s *MainTestSuite) TestHandleAllocationEvent_ReportsOOMKilledStatus() {
	restartedAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	event := nomadApi.TaskEvent{Details: map[string]string{"oom_killed": "true"}}
	state := nomadApi.TaskState{Restarts: 1, Events: []*nomadApi.TaskEvent{&event}}
	restartedAllocation.TaskStates = map[string]*nomadApi.TaskState{TaskName: &state}
	restartedEvent := eventForAllocation(s.T(), restartedAllocation)

	allocations := storage.NewLocalStorage[*allocationData]()
	allocations.Add(restartedAllocation.ID, &allocationData{jobID: restartedAllocation.JobID})

	var reason error
	err := handleAllocationEvent(time.Now().UnixNano(), allocations, &restartedEvent, &AllocationProcessing{
		OnNew: func(_ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ string, r error) bool {
			reason = r
			return true
		},
	})
	s.Require().NoError(err)
	s.ErrorIs(reason, ErrorOOMKilled)
}

func (s *MainTestSuite) TestAPIClient_WatchAllocationsReturnsErrorWhenAllocationStreamCannotBeRetrieved() {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.Anything).Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, map[string]chan error{}, storage.NewLocalStorage[*allocationData](), false}

	err := apiClient.WatchEventStream(context.Background(), noopAllocationProcessing)
	s.ErrorIs(err, tests.ErrDefault)
}

// Test case: WatchAllocations returns an error when an allocation cannot be retrieved without receiving further events.
func (s *MainTestSuite) TestAPIClient_WatchAllocations() {
	event := nomadApi.Event{
		Type:  structs.TypeAllocationUpdated,
		Topic: nomadApi.TopicAllocation,
		// This should fail decoding, as Allocation.ID is expected to be a string, not int
		Payload: map[string]interface{}{"Allocation": map[string]interface{}{"ID": 1}},
	}
	_, err := event.Allocation()
	s.Require().Error(err)

	events := []*nomadApi.Events{{Events: []nomadApi.Event{event}}, {}}
	eventsProcessed, err := runAllocationWatching(s.T(), events, noopAllocationProcessing)
	s.Error(err)
	s.Equal(1, eventsProcessed)
}

func (s *MainTestSuite) TestAPIClient_WatchAllocationsReturnsErrorOnUnexpectedEOF() {
	events := []*nomadApi.Events{{Err: ErrUnexpectedEOF}, {}}
	eventsProcessed, err := runAllocationWatching(s.T(), events, noopAllocationProcessing)
	s.Error(err)
	s.Equal(1, eventsProcessed)
}

func assertWatchAllocation(t *testing.T, events []*nomadApi.Events,
	expectedNewAllocations []*nomadApi.Allocation, expectedDeletedAllocations []string) {
	t.Helper()
	var newAllocations []*nomadApi.Allocation
	var deletedAllocations []string
	callbacks := &AllocationProcessing{
		OnNew: func(alloc *nomadApi.Allocation, _ time.Duration) {
			newAllocations = append(newAllocations, alloc)
		},
		OnDeleted: func(jobID string, _ error) bool {
			deletedAllocations = append(deletedAllocations, jobID)
			return false
		},
	}

	eventsProcessed, err := runAllocationWatching(t, events, callbacks)
	assert.NoError(t, err)
	assert.Equal(t, len(events), eventsProcessed)

	assert.Equal(t, expectedNewAllocations, newAllocations)
	assert.Equal(t, expectedDeletedAllocations, deletedAllocations)
}

// runAllocationWatching simulates events streamed from the Nomad event stream
// to the MonitorEvaluation method. It starts the MonitorEvaluation function as a goroutine
// and sequentially transfers the events from the given array to a channel simulating the stream.
func runAllocationWatching(t *testing.T, events []*nomadApi.Events, callbacks *AllocationProcessing) (
	eventsProcessed int, err error) {
	t.Helper()
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyWatchAllocations(stream, callbacks)
	return simulateNomadEventStream(stream, errChan, events)
}

// asynchronouslyMonitorEvaluation creates an APIClient with mocked Nomad API and
// runs the MonitorEvaluation method in a goroutine. The mock returns a read-only
// version of the given stream to simulate an event stream gotten from the real
// Nomad API.
func asynchronouslyWatchAllocations(stream chan *nomadApi.Events, callbacks *AllocationProcessing) chan error {
	ctx := context.Background()
	// We can only get a read-only channel once we return it from a function.
	readOnlyStream := func() <-chan *nomadApi.Events { return stream }()

	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", ctx).Return(readOnlyStream, nil)
	apiClient := &APIClient{apiMock, map[string]chan error{}, storage.NewLocalStorage[*allocationData](), false}

	errChan := make(chan error)
	go func() {
		errChan <- apiClient.WatchEventStream(ctx, callbacks)
	}()
	return errChan
}

// eventForEvaluation takes an evaluation and creates an Event with the given evaluation
// as its payload. Nomad uses the mapstructure library to decode the payload, which we
// simply reverse here.
func eventForAllocation(t *testing.T, alloc *nomadApi.Allocation) nomadApi.Event {
	t.Helper()
	payload := make(map[string]interface{})

	err := mapstructure.Decode(eventPayload{Allocation: alloc}, &payload)
	if err != nil {
		t.Fatalf("Couldn't decode allocation %v into payload map", err)
		return nomadApi.Event{}
	}
	event := nomadApi.Event{
		Topic:   nomadApi.TopicAllocation,
		Type:    structs.TypeAllocationUpdated,
		Payload: payload,
	}
	return event
}

func createAllocation(modifyTime int64, clientStatus, desiredStatus string) *nomadApi.Allocation {
	return &nomadApi.Allocation{
		ID:            tests.DefaultUUID,
		JobID:         tests.DefaultRunnerID,
		ModifyTime:    modifyTime,
		ClientStatus:  clientStatus,
		DesiredStatus: desiredStatus,
	}
}

func createRecentAllocation(clientStatus, desiredStatus string) *nomadApi.Allocation {
	return createAllocation(time.Now().Add(time.Minute).UnixNano(), clientStatus, desiredStatus)
}

func TestExecuteCommandTestSuite(t *testing.T) {
	suite.Run(t, new(ExecuteCommandTestSuite))
}

type ExecuteCommandTestSuite struct {
	tests.MemoryLeakTestSuite
	allocationID   string
	ctx            context.Context
	testCommand    string
	expectedStdout string
	expectedStderr string
	apiMock        *apiQuerierMock
	nomadAPIClient APIClient
}

func (s *ExecuteCommandTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.allocationID = "test-allocation-id"
	s.ctx = context.Background()
	s.testCommand = "echo \"do nothing\""
	s.expectedStdout = "stdout"
	s.expectedStderr = "stderr"
	s.apiMock = &apiQuerierMock{}
	s.nomadAPIClient = APIClient{apiQuerier: s.apiMock}
}

const withTTY = true

func (s *ExecuteCommandTestSuite) TestWithSeparateStderr() {
	config.Config.Server.InteractiveStderr = true
	commandExitCode := 42
	stderrExitCode := 1

	var stdout, stderr bytes.Buffer
	var calledStdoutCommand, calledStderrCommand string

	// mock regular call
	call := s.mockExecute(mock.AnythingOfType("string"), 0, nil, func(_ mock.Arguments) {})
	call.Run(func(args mock.Arguments) {
		var ok bool
		calledCommand, ok := args.Get(2).(string)
		s.Require().True(ok)
		writer, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)

		if isStderrCommand := strings.Contains(calledCommand, "mkfifo"); isStderrCommand {
			calledStderrCommand = calledCommand
			_, err := writer.Write([]byte(s.expectedStderr))
			s.Require().NoError(err)
			call.ReturnArguments = mock.Arguments{stderrExitCode, nil}
		} else {
			calledStdoutCommand = calledCommand
			_, err := writer.Write([]byte(s.expectedStdout))
			s.Require().NoError(err)
			call.ReturnArguments = mock.Arguments{commandExitCode, nil}
		}
	})

	exitCode, err := s.nomadAPIClient.ExecuteCommand(s.allocationID, s.ctx, s.testCommand, withTTY,
		UnprivilegedExecution, nullio.Reader{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 2)
	s.Equal(commandExitCode, exitCode)

	s.Run("should wrap command in stderr wrapper", func() {
		s.Require().NotEmpty(calledStdoutCommand)
		stderrWrapperCommand := fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoFormat, s.testCommand, stderrFifoFormat)
		stdoutFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrWrapperCommand), "%d", "\\d*")
		stdoutFifoRegexp = strings.Replace(stdoutFifoRegexp, s.testCommand, ".*", 1)
		s.Regexp(stdoutFifoRegexp, calledStdoutCommand)
	})

	s.Run("should call correct stderr command", func() {
		s.Require().NotEmpty(calledStderrCommand)
		stderrFifoCommand := fmt.Sprintf(stderrFifoCommandFormat, stderrFifoFormat, stderrFifoFormat, stderrFifoFormat)
		stderrFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrFifoCommand), "%d", "\\d*")
		s.Regexp(stderrFifoRegexp, calledStderrCommand)
	})

	s.Run("should return correct output", func() {
		s.Equal(s.expectedStdout, stdout.String())
		s.Equal(s.expectedStderr, stderr.String())
	})
}

func (s *ExecuteCommandTestSuite) TestWithSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = true

	call := s.mockExecute(mock.AnythingOfType("string"), 0, nil, func(_ mock.Arguments) {})
	call.Run(func(args mock.Arguments) {
		var ok bool
		calledCommand, ok := args.Get(2).(string)
		s.Require().True(ok)

		if isStderrCommand := strings.Contains(calledCommand, "mkfifo"); isStderrCommand {
			// Here we defuse the data race condition of the ReturnArguments being set twice at the same time.
			<-time.After(tests.ShortTimeout)
			call.ReturnArguments = mock.Arguments{1, nil}
		} else {
			call.ReturnArguments = mock.Arguments{1, tests.ErrDefault}
		}
	})

	_, err := s.nomadAPIClient.ExecuteCommand(s.allocationID, s.ctx, s.testCommand, withTTY, UnprivilegedExecution,
		nullio.Reader{}, io.Discard, io.Discard)
	s.Equal(tests.ErrDefault, err)
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderr() {
	config.Config.Server.InteractiveStderr = false
	var stdout, stderr bytes.Buffer
	commandExitCode := 42

	// mock regular call
	expectedCommand := prepareCommandWithoutTTY(s.testCommand, UnprivilegedExecution)
	s.mockExecute(expectedCommand, commandExitCode, nil, func(args mock.Arguments) {
		stdout, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := stdout.Write([]byte(s.expectedStdout))
		s.Require().NoError(err)
		stderr, ok := args.Get(6).(io.Writer)
		s.Require().True(ok)
		_, err = stderr.Write([]byte(s.expectedStderr))
		s.Require().NoError(err)
	})

	exitCode, err := s.nomadAPIClient.ExecuteCommand(s.allocationID, s.ctx, s.testCommand, withTTY,
		UnprivilegedExecution, nullio.Reader{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 1)
	s.Equal(commandExitCode, exitCode)
	s.Equal(s.expectedStdout, stdout.String())
	s.Equal(s.expectedStderr, stderr.String())
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = false
	expectedCommand := prepareCommandWithoutTTY(s.testCommand, UnprivilegedExecution)
	s.mockExecute(expectedCommand, 1, tests.ErrDefault, func(args mock.Arguments) {})
	_, err := s.nomadAPIClient.ExecuteCommand(s.allocationID, s.ctx, s.testCommand, withTTY, UnprivilegedExecution,
		nullio.Reader{}, io.Discard, io.Discard)
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *ExecuteCommandTestSuite) mockExecute(command interface{}, exitCode int,
	err error, runFunc func(arguments mock.Arguments)) *mock.Call {
	return s.apiMock.On("Execute", s.allocationID, mock.Anything, command, withTTY,
		mock.Anything, mock.Anything, mock.Anything).
		Run(runFunc).
		Return(exitCode, err)
}

func (s *MainTestSuite) TestAPIClient_LoadRunnerPortMappings() {
	apiMock := &apiQuerierMock{}
	mockedCall := apiMock.On("allocation", tests.DefaultRunnerID)
	nomadAPIClient := APIClient{apiQuerier: apiMock}

	s.Run("should return error when API query fails", func() {
		mockedCall.Return(nil, tests.ErrDefault)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		s.Nil(portMappings)
		s.ErrorIs(err, tests.ErrDefault)
	})

	s.Run("should return error when AllocatedResources is nil", func() {
		mockedCall.Return(&nomadApi.Allocation{AllocatedResources: nil}, nil)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		s.ErrorIs(err, ErrorNoAllocatedResourcesFound)
		s.Nil(portMappings)
	})

	s.Run("should correctly return ports", func() {
		allocation := &nomadApi.Allocation{
			AllocatedResources: &nomadApi.AllocatedResources{
				Shared: nomadApi.AllocatedSharedResources{Ports: tests.DefaultPortMappings},
			},
		}
		mockedCall.Return(allocation, nil)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		s.NoError(err)
		s.Equal(tests.DefaultPortMappings, portMappings)
	})
}
