package nomad

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
)

type eventPayload struct {
	Evaluation *nomadApi.Evaluation
	Allocation *nomadApi.Allocation
}

var (
	noopAllocationProcessing = &AllocationProcessing{
		OnNew:     func(_ context.Context, _ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ context.Context, _ string, _ error) bool { return false },
	}
	ErrUnexpectedEOF = errors.New("unexpected EOF")
)

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
	s.Require().NoError(err)
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationReturnsErrorWhenStreamReturnsError() {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.AnythingOfType("*context.cancelCtx")).
		Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, storage.NewLocalStorage[chan error](), storage.NewLocalStorage[*allocationData](), false}
	err := apiClient.MonitorEvaluation(context.Background(), "id")
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationWithSuccessfulEvent() {
	eval := nomadApi.Evaluation{Status: structs.EvalStatusComplete}
	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	// make sure that the tested function can complete
	s.Require().NoError(checkEvaluation(&eval))

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(s.T(), &pendingEval), eventForEvaluation(s.T(), &eval),
	}}

	cases := []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		name                    string
	}{
		{
			[]*nomadApi.Events{&events},
			1,
			"it completes with successful event",
		},
		{
			[]*nomadApi.Events{&events, &events},
			2,
			"it keeps listening after first successful event",
		},
		{
			[]*nomadApi.Events{{}, &events},
			2,
			"it skips heartbeat and completes",
		},
		{
			[]*nomadApi.Events{&pendingEvaluationEvents, &events},
			2,
			"it skips pending evaluation and completes",
		},
		{
			[]*nomadApi.Events{&multipleEventsWithPending},
			1,
			"it handles multiple events per received event",
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			eventsProcessed, err := runEvaluationMonitoring(s.TestCtx, c.streamedEvents)
			s.Require().NoError(err)
			s.Equal(c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func (s *MainTestSuite) TestApiClient_MonitorEvaluationWithFailingEvent() {
	eval := nomadApi.Evaluation{ID: evaluationID, Status: structs.EvalStatusFailed}
	evalErr := checkEvaluation(&eval)
	s.Require().Error(evalErr)

	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(s.T(), &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(s.T(), &pendingEval), eventForEvaluation(s.T(), &eval),
	}}
	eventsWithErr := nomadApi.Events{Err: tests.ErrDefault, Events: []nomadApi.Event{{}}}

	cases := []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		expectedError           error
		name                    string
	}{
		{
			[]*nomadApi.Events{&events},
			1, evalErr,
			"it fails with failing event",
		},
		{
			[]*nomadApi.Events{{}, &events},
			2, evalErr,
			"it skips heartbeat and fail",
		},
		{
			[]*nomadApi.Events{&pendingEvaluationEvents, &events},
			2, evalErr,
			"it skips pending evaluation and fail",
		},
		{
			[]*nomadApi.Events{&multipleEventsWithPending},
			1, evalErr,
			"it handles multiple events per received event and fails",
		},
		{
			[]*nomadApi.Events{&eventsWithErr},
			1, tests.ErrDefault,
			"it fails with event error when event has error",
		},
	}

	for _, c := range cases {
		s.Run(c.name, func() {
			eventsProcessed, err := runEvaluationMonitoring(s.TestCtx, c.streamedEvents)
			s.Require().Error(err)
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
	s.Require().Error(err)
	eventsProcessed, err := runEvaluationMonitoring(s.TestCtx, []*nomadApi.Events{{Events: []nomadApi.Event{event}}})
	s.Require().Error(err)
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
		s.Require().Error(err)
		msgWithoutBlockedEval = err.Error()
		assertMessageContainsCorrectStrings(msgWithoutBlockedEval)
	})

	s.Run("with blocked eval", func() {
		evaluation.BlockedEval = "blocking-eval"
		err := checkEvaluation(&evaluation)
		s.Require().Error(err)
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
		s.Require().NoError(err)
	})

	s.Run("when evaluation status not complete", func() {
		incompleteStates := []string{
			structs.EvalStatusFailed, structs.EvalStatusCancelled,
			structs.EvalStatusBlocked, structs.EvalStatusPending,
		}
		for _, status := range incompleteStates {
			evaluation.Status = status
			err := checkEvaluation(&evaluation)
			s.Require().Error(err)
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

	assertWatchAllocation(s, []*nomadApi.Events{&oldAllocationEvents},
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
	assertWatchAllocation(s, []*nomadApi.Events{&nodeEvents}, []*nomadApi.Allocation(nil), []string(nil))
}

func (s *MainTestSuite) TestApiClient_WatchAllocationsUsesCallbacksForEvents() {
	pendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	pendingEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), pendingAllocation)}}

	s.Run("it does not add allocation when client status is pending", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingEvents}, []*nomadApi.Allocation(nil), []string(nil))
	})

	startedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	startedEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), startedAllocation)}}
	pendingStartedEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), pendingAllocation), eventForAllocation(s.T(), startedAllocation),
	}}

	s.Run("it adds allocation with matching events", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})

	s.Run("it skips heartbeat and adds allocation with matching events", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})

	stoppingAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusStop)
	stoppingEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), stoppingAllocation)}}
	pendingStartStoppingEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), pendingAllocation),
		eventForAllocation(s.T(), startedAllocation),
		eventForAllocation(s.T(), stoppingAllocation),
	}}

	s.Run("it adds and deletes the allocation", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartStoppingEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{stoppingAllocation.JobID})
	})

	s.Run("it ignores duplicate events", func() {
		assertWatchAllocation(s, []*nomadApi.Events{
			&pendingEvents, &startedEvents, &startedEvents,
			&stoppingEvents, &stoppingEvents, &stoppingEvents,
		},
			[]*nomadApi.Allocation{startedAllocation}, []string{startedAllocation.JobID})
	})

	s.Run("it ignores events of unknown allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{
			&startedEvents, &startedEvents,
			&stoppingEvents, &stoppingEvents, &stoppingEvents,
		}, []*nomadApi.Allocation(nil), []string(nil))
	})

	s.Run("it removes restarted allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents, &pendingStartedEvents},
			[]*nomadApi.Allocation{startedAllocation, startedAllocation}, []string{startedAllocation.JobID})
	})

	rescheduleAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	rescheduleAllocation.ID = tests.AnotherUUID
	rescheduleAllocation.PreviousAllocation = pendingAllocation.ID
	rescheduleStartedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	rescheduleStartedAllocation.ID = tests.AnotherUUID
	rescheduleAllocation.PreviousAllocation = pendingAllocation.ID
	rescheduleEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), rescheduleAllocation), eventForAllocation(s.T(), rescheduleStartedAllocation),
	}}

	s.Run("it removes rescheduled allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents, &rescheduleEvents},
			[]*nomadApi.Allocation{startedAllocation, rescheduleStartedAllocation}, []string{startedAllocation.JobID})
	})

	stoppedPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusStop)
	stoppedPendingEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), stoppedPendingAllocation)}}

	s.Run("it does not callback for stopped pending allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingEvents, &stoppedPendingEvents},
			[]*nomadApi.Allocation(nil), []string(nil))
	})

	failedAllocation := createRecentAllocation(structs.AllocClientStatusFailed, structs.AllocDesiredStatusStop)
	failedEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), failedAllocation)}}

	s.Run("it removes stopped failed allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents, &failedEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{failedAllocation.JobID})
	})

	lostAllocation := createRecentAllocation(structs.AllocClientStatusLost, structs.AllocDesiredStatusStop)
	lostEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(s.T(), lostAllocation)}}

	s.Run("it removes stopped lost allocations", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents, &lostEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string{lostAllocation.JobID})
	})

	rescheduledLostAllocation := createRecentAllocation(structs.AllocClientStatusLost, structs.AllocDesiredStatusStop)
	rescheduledLostAllocation.NextAllocation = tests.AnotherUUID
	rescheduledLostEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(s.T(), rescheduledLostAllocation),
	}}

	s.Run("it removes lost allocations not before the last restart attempt", func() {
		assertWatchAllocation(s, []*nomadApi.Events{&pendingStartedEvents, &rescheduledLostEvents},
			[]*nomadApi.Allocation{startedAllocation}, []string(nil))
	})
}

func (s *MainTestSuite) TestHandleAllocationEventBuffersPendingAllocation() {
	s.Run("AllocationUpdated", func() {
		newPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
		newPendingEvent := eventForAllocation(s.T(), newPendingAllocation)

		allocations := storage.NewLocalStorage[*allocationData]()
		err := handleAllocationEvent(s.TestCtx,
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
		err := handleAllocationEvent(s.TestCtx,
			time.Now().UnixNano(), allocations, &newPendingEvent, noopAllocationProcessing)
		s.Require().NoError(err)

		_, ok := allocations.Get(newPendingAllocation.ID)
		s.True(ok)
	})
}

func (s *MainTestSuite) TestHandleAllocationEvent_RegressionTest_14_09_2023() {
	jobID := "29-6f04b525-5315-11ee-af32-fa163e079f19"
	a1ID := "04d86250-550c-62f9-9a21-ecdc3b38773e"
	a2ID := "102f282f-376a-1453-4d3d-7d4e32046acd"
	a3ID := "0d8a8ece-cf52-2968-5a9f-e972a4150a6e"

	a1Starting := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	// With this event the job is added to the idle runners
	a1Running := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)

	// With this event the job is removed from the idle runners
	a2Starting := createRecentAllocationWithID(a2ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	a2Starting.PreviousAllocation = a1ID

	// Because the runner is neither an idle runner nor a used runner, this event triggered the now removed
	// race condition handling that led to neither removing a2 from the allocations nor adding a3 to the allocations.
	a3Starting := createRecentAllocationWithID(a3ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	a3Starting.PreviousAllocation = a2ID

	// a2Stopping was not ignored and led to an unexpected allocation stopping.
	a2Stopping := createRecentAllocationWithID(a2ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusStop)
	a2Stopping.PreviousAllocation = a1ID
	a2Stopping.NextAllocation = a3ID

	// a2Complete was not ignored (wrong behavior).
	a2Complete := createRecentAllocationWithID(a2ID, jobID, structs.AllocClientStatusComplete, structs.AllocDesiredStatusStop)
	a2Complete.PreviousAllocation = a1ID
	a2Complete.NextAllocation = a3ID

	// a3Running was ignored because it was unknown (wrong behavior).
	a3Running := createRecentAllocationWithID(a3ID, jobID, structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	a3Running.PreviousAllocation = a2ID

	events := []*nomadApi.Events{{Events: []nomadApi.Event{
		eventForAllocation(s.T(), a1Starting),
		eventForAllocation(s.T(), a1Running),
		eventForAllocation(s.T(), a2Starting),
		eventForAllocation(s.T(), a3Starting),
		eventForAllocation(s.T(), a2Stopping),
		eventForAllocation(s.T(), a2Complete),
		eventForAllocation(s.T(), a3Running),
	}}}
	assertAllocationRunning(s, events, []string{jobID})
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
	err := handleAllocationEvent(s.TestCtx, time.Now().UnixNano(), allocations, &restartedEvent, &AllocationProcessing{
		OnNew: func(_ context.Context, _ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ context.Context, _ string, r error) bool {
			reason = r
			return true
		},
	})
	s.Require().NoError(err)
	s.ErrorIs(reason, ErrOOMKilled)
}

func (s *MainTestSuite) TestAPIClient_WatchAllocationsReturnsErrorWhenAllocationStreamCannotBeRetrieved() {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.Anything).Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, storage.NewLocalStorage[chan error](), storage.NewLocalStorage[*allocationData](), false}

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
	eventsProcessed, err := runAllocationWatching(s, events, noopAllocationProcessing)
	s.Require().Error(err)
	s.Equal(1, eventsProcessed)
}

func (s *MainTestSuite) TestAPIClient_WatchAllocationsReturnsErrorOnUnexpectedEOF() {
	events := []*nomadApi.Events{{Err: ErrUnexpectedEOF}, {}}
	eventsProcessed, err := runAllocationWatching(s, events, noopAllocationProcessing)
	s.Require().Error(err)
	s.Equal(1, eventsProcessed)
}

func (s *MainTestSuite) TestAPIClient_WatchAllocationsCanHandleMigration() {
	jobID := "10-0331c7d8-03c1-11ef-b832-fa163e7afdf8"
	a1ID := "84a734a1-5573-6116-5678-86060ce4c479"
	a2ID := "28e08715-38a9-42b3-8f77-0a14ee68b482"

	a1Starting := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	a1Running := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	a1Stopping := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusRunning, structs.AllocDesiredStatusStop)

	a2Starting := createRecentAllocationWithID(a2ID, jobID, structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	a2Running := createRecentAllocationWithID(a2ID, jobID, structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)

	a1Complete := createRecentAllocationWithID(a1ID, jobID, structs.AllocClientStatusComplete, structs.AllocDesiredStatusStop)

	events := []*nomadApi.Events{{Events: []nomadApi.Event{
		eventForAllocation(s.T(), a1Starting),
		eventForAllocation(s.T(), a1Running),
		eventForAllocation(s.T(), a1Stopping),
		eventForAllocation(s.T(), a2Starting),
		eventForAllocation(s.T(), a2Running),
		eventForAllocation(s.T(), a1Complete),
	}}}
	assertAllocationRunning(s, events, []string{jobID})
}

// assertWatchAllocation simulates the passed events and compares the expected new/deleted allocations with the actual
// new/deleted allocations.
func assertWatchAllocation(s *MainTestSuite, events []*nomadApi.Events,
	expectedNewAllocations []*nomadApi.Allocation, expectedDeletedAllocations []string,
) {
	s.T().Helper()
	var newAllocations []*nomadApi.Allocation
	var deletedAllocations []string
	callbacks := &AllocationProcessing{
		OnNew: func(_ context.Context, alloc *nomadApi.Allocation, _ time.Duration) {
			newAllocations = append(newAllocations, alloc)
		},
		OnDeleted: func(_ context.Context, jobID string, _ error) bool {
			deletedAllocations = append(deletedAllocations, jobID)
			return false
		},
	}

	eventsProcessed, err := runAllocationWatching(s, events, callbacks)
	s.Require().NoError(err)
	s.Equal(len(events), eventsProcessed)

	s.Equal(expectedNewAllocations, newAllocations)
	s.Equal(expectedDeletedAllocations, deletedAllocations)
}

// assertAllocationRunning simulates the passed events and asserts that the runners with an id in the runningIDs are
// active in the end.
func assertAllocationRunning(s *MainTestSuite, events []*nomadApi.Events, runningIDs []string) {
	activeAllocations := make(map[string]bool)
	callbacks := &AllocationProcessing{
		OnNew: func(_ context.Context, alloc *nomadApi.Allocation, _ time.Duration) {
			activeAllocations[alloc.JobID] = true
		},
		OnDeleted: func(_ context.Context, jobID string, _ error) bool {
			_, ok := activeAllocations[jobID]
			delete(activeAllocations, jobID)
			return !ok
		},
	}

	_, err := runAllocationWatching(s, events, callbacks)
	s.Require().NoError(err)

	for _, id := range runningIDs {
		s.True(activeAllocations[id], id)
	}
}

// runAllocationWatching simulates events streamed from the Nomad event stream
// to the MonitorEvaluation method. It starts the MonitorEvaluation function as a goroutine
// and sequentially transfers the events from the given array to a channel simulating the stream.
func runAllocationWatching(s *MainTestSuite, events []*nomadApi.Events, callbacks *AllocationProcessing) (
	eventsProcessed int, err error,
) {
	s.T().Helper()
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyWatchAllocations(stream, callbacks)
	return simulateNomadEventStream(s.TestCtx, stream, errChan, events)
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
	apiClient := &APIClient{apiMock, storage.NewLocalStorage[chan error](), storage.NewLocalStorage[*allocationData](), false}

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

func createRecentAllocationWithID(allocID, jobID, clientStatus, desiredStatus string) *nomadApi.Allocation {
	alloc := createAllocation(time.Now().Add(time.Minute).UnixNano(), clientStatus, desiredStatus)
	alloc.ID = allocID
	alloc.JobID = jobID
	return alloc
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
	ctx context.Context,
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
	case <-ctx.Done():
	case err = <-errChan:
	}
	return eventsProcessed, err
}

// runEvaluationMonitoring simulates events streamed from the Nomad event stream
// to the MonitorEvaluation method. It starts the MonitorEvaluation function as a goroutine
// and sequentially transfers the events from the given array to a channel simulating the stream.
func runEvaluationMonitoring(ctx context.Context, events []*nomadApi.Events) (eventsProcessed int, err error) {
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyMonitorEvaluation(stream)
	return simulateNomadEventStream(ctx, stream, errChan, events)
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
	apiClient := &APIClient{apiMock, storage.NewLocalStorage[chan error](), storage.NewLocalStorage[*allocationData](), false}

	errChan := make(chan error)
	go func() {
		errChan <- apiClient.MonitorEvaluation(ctx, evaluationID)
	}()
	return errChan
}
