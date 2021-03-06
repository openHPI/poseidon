package nomad

import (
	"bytes"
	"context"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	noopAllocationProcessoring = &AllocationProcessoring{
		OnNew:     func(_ *nomadApi.Allocation, _ time.Duration) {},
		OnDeleted: func(_ *nomadApi.Allocation) {},
	}
)

func TestLoadRunnersTestSuite(t *testing.T) {
	suite.Run(t, new(LoadRunnersTestSuite))
}

type LoadRunnersTestSuite struct {
	suite.Suite
	jobID                  string
	mock                   *apiQuerierMock
	nomadAPIClient         APIClient
	availableRunner        *nomadApi.JobListStub
	anotherAvailableRunner *nomadApi.JobListStub
	pendingRunner          *nomadApi.JobListStub
	deadRunner             *nomadApi.JobListStub
}

func (s *LoadRunnersTestSuite) SetupTest() {
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

func TestApiClient_init(t *testing.T) {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(NomadTestConfig(TestDefaultAddress))
	require.Nil(t, err)
}

func TestApiClientCanNotBeInitializedWithInvalidUrl(t *testing.T) {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(NomadTestConfig("http://" + TestDefaultAddress))
	assert.NotNil(t, err)
}

func TestNewExecutorApiCanBeCreatedWithoutError(t *testing.T) {
	expectedClient := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := expectedClient.init(NomadTestConfig(TestDefaultAddress))
	require.Nil(t, err)

	_, err = NewExecutorAPI(NomadTestConfig(TestDefaultAddress))
	require.Nil(t, err)
}

// asynchronouslyMonitorEvaluation creates an APIClient with mocked Nomad API and
// runs the MonitorEvaluation method in a goroutine. The mock returns a read-only
// version of the given stream to simulate an event stream gotten from the real
// Nomad API.
func asynchronouslyMonitorEvaluation(stream chan *nomadApi.Events) chan error {
	ctx := context.Background()
	// We can only get a read-only channel once we return it from a function.
	readOnlyStream := func() <-chan *nomadApi.Events { return stream }()

	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.AnythingOfType("*context.cancelCtx")).
		Return(readOnlyStream, nil)
	apiClient := &APIClient{apiMock, map[string]chan error{}, false}

	errChan := make(chan error)
	go func() {
		errChan <- apiClient.MonitorEvaluation(evaluationID, ctx)
	}()
	return errChan
}

func TestApiClient_MonitorEvaluationReturnsNilWhenStreamIsClosed(t *testing.T) {
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyMonitorEvaluation(stream)

	close(stream)
	var err error
	// If close doesn't terminate MonitorEvaluation, this test won't complete without a timeout.
	select {
	case err = <-errChan:
	case <-time.After(time.Millisecond * 10):
		t.Fatal("MonitorEvaluation didn't finish as expected")
	}
	assert.Nil(t, err)
}

func TestApiClient_MonitorEvaluationReturnsErrorWhenStreamReturnsError(t *testing.T) {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.AnythingOfType("*context.cancelCtx")).
		Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, map[string]chan error{}, false}
	err := apiClient.MonitorEvaluation("id", context.Background())
	assert.ErrorIs(t, err, tests.ErrDefault)
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
	stream chan *nomadApi.Events,
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

func TestApiClient_MonitorEvaluationWithSuccessfulEvent(t *testing.T) {
	eval := nomadApi.Evaluation{Status: structs.EvalStatusComplete}
	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	// make sure that the tested function can complete
	require.Nil(t, checkEvaluation(&eval))

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(t, &pendingEval), eventForEvaluation(t, &eval),
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
		t.Run(c.name, func(t *testing.T) {
			eventsProcessed, err := runEvaluationMonitoring(c.streamedEvents)
			assert.Nil(t, err)
			assert.Equal(t, c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func TestApiClient_MonitorEvaluationWithFailingEvent(t *testing.T) {
	eval := nomadApi.Evaluation{ID: evaluationID, Status: structs.EvalStatusFailed}
	evalErr := checkEvaluation(&eval)
	require.NotNil(t, evalErr)

	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, &eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, &pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(t, &pendingEval), eventForEvaluation(t, &eval),
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
		t.Run(c.name, func(t *testing.T) {
			eventsProcessed, err := runEvaluationMonitoring(c.streamedEvents)
			require.NotNil(t, err)
			assert.Contains(t, err.Error(), c.expectedError.Error())
			assert.Equal(t, c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func TestApiClient_MonitorEvaluationFailsWhenFailingToDecodeEvaluation(t *testing.T) {
	event := nomadApi.Event{
		Topic: nomadApi.TopicEvaluation,
		// This should fail decoding, as Evaluation.Status is expected to be a string, not int
		Payload: map[string]interface{}{"Evaluation": map[string]interface{}{"Status": 1}},
	}
	_, err := event.Evaluation()
	require.NotNil(t, err)
	eventsProcessed, err := runEvaluationMonitoring([]*nomadApi.Events{{Events: []nomadApi.Event{event}}})
	assert.Error(t, err)
	assert.Equal(t, 1, eventsProcessed)
}

func TestCheckEvaluationWithFailedAllocations(t *testing.T) {
	testKey := "test1"
	failedAllocs := map[string]*nomadApi.AllocationMetric{
		testKey: {NodesExhausted: 1},
	}
	evaluation := nomadApi.Evaluation{FailedTGAllocs: failedAllocs, Status: structs.EvalStatusFailed}

	assertMessageContainsCorrectStrings := func(msg string) {
		assert.Contains(t, msg, evaluation.Status, "error should contain the evaluation status")
		assert.Contains(t, msg, fmt.Sprintf("%s: %#v", testKey, failedAllocs[testKey]),
			"error should contain the failed allocations metric")
	}

	var msgWithoutBlockedEval, msgWithBlockedEval string
	t.Run("without blocked eval", func(t *testing.T) {
		err := checkEvaluation(&evaluation)
		require.NotNil(t, err)
		msgWithoutBlockedEval = err.Error()
		assertMessageContainsCorrectStrings(msgWithoutBlockedEval)
	})

	t.Run("with blocked eval", func(t *testing.T) {
		evaluation.BlockedEval = "blocking-eval"
		err := checkEvaluation(&evaluation)
		require.NotNil(t, err)
		msgWithBlockedEval = err.Error()
		assertMessageContainsCorrectStrings(msgWithBlockedEval)
	})

	assert.NotEqual(t, msgWithBlockedEval, msgWithoutBlockedEval)
}

func TestCheckEvaluationWithoutFailedAllocations(t *testing.T) {
	evaluation := nomadApi.Evaluation{FailedTGAllocs: make(map[string]*nomadApi.AllocationMetric)}

	t.Run("when evaluation status complete", func(t *testing.T) {
		evaluation.Status = structs.EvalStatusComplete
		err := checkEvaluation(&evaluation)
		assert.Nil(t, err)
	})

	t.Run("when evaluation status not complete", func(t *testing.T) {
		incompleteStates := []string{structs.EvalStatusFailed, structs.EvalStatusCancelled,
			structs.EvalStatusBlocked, structs.EvalStatusPending}
		for _, status := range incompleteStates {
			evaluation.Status = status
			err := checkEvaluation(&evaluation)
			require.NotNil(t, err)
			assert.Contains(t, err.Error(), status, "error should contain the evaluation status")
		}
	})
}

func TestApiClient_WatchAllocationsIgnoresOldAllocations(t *testing.T) {
	oldStoppedAllocation := createOldAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusStop)
	oldPendingAllocation := createOldAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	oldRunningAllocation := createOldAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	oldAllocationEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(t, oldStoppedAllocation),
		eventForAllocation(t, oldPendingAllocation),
		eventForAllocation(t, oldRunningAllocation),
	}}

	assertWatchAllocation(t, []*nomadApi.Events{&oldAllocationEvents},
		[]*nomadApi.Allocation(nil), []*nomadApi.Allocation(nil))
}

func createOldAllocation(clientStatus, desiredStatus string) *nomadApi.Allocation {
	return createAllocation(time.Now().Add(-time.Minute).UnixNano(), clientStatus, desiredStatus)
}

func TestApiClient_WatchAllocationsIgnoresUnhandledEvents(t *testing.T) {
	nodeEvents := nomadApi.Events{Events: []nomadApi.Event{
		{
			Topic: nomadApi.TopicNode,
			Type:  structs.TypeNodeEvent,
		},
	}}
	assertWatchAllocation(t, []*nomadApi.Events{&nodeEvents}, []*nomadApi.Allocation(nil), []*nomadApi.Allocation(nil))

	planEvents := nomadApi.Events{Events: []nomadApi.Event{
		{
			Topic: nomadApi.TopicAllocation,
			Type:  structs.TypePlanResult,
		},
	}}
	assertWatchAllocation(t, []*nomadApi.Events{&planEvents}, []*nomadApi.Allocation(nil), []*nomadApi.Allocation(nil))
}

func TestApiClient_WatchAllocationsHandlesEvents(t *testing.T) {
	newPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	pendingAllocationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForAllocation(t, newPendingAllocation)}}

	newStartedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusRun)
	startAllocationEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(t, newPendingAllocation),
		eventForAllocation(t, newStartedAllocation),
	}}

	newStoppedAllocation := createRecentAllocation(structs.AllocClientStatusRunning, structs.AllocDesiredStatusStop)
	stopAllocationEvents := nomadApi.Events{Events: []nomadApi.Event{
		eventForAllocation(t, newPendingAllocation),
		eventForAllocation(t, newStartedAllocation),
		eventForAllocation(t, newStoppedAllocation),
	}}

	var cases = []struct {
		streamedEvents             []*nomadApi.Events
		expectedNewAllocations     []*nomadApi.Allocation
		expectedDeletedAllocations []*nomadApi.Allocation
		name                       string
	}{
		{[]*nomadApi.Events{&pendingAllocationEvents},
			[]*nomadApi.Allocation(nil), []*nomadApi.Allocation(nil),
			"it does not add allocation when client status is pending"},
		{[]*nomadApi.Events{&startAllocationEvents},
			[]*nomadApi.Allocation{newStartedAllocation},
			[]*nomadApi.Allocation(nil),
			"it adds allocation with matching events"},
		{[]*nomadApi.Events{{}, &startAllocationEvents},
			[]*nomadApi.Allocation{newStartedAllocation},
			[]*nomadApi.Allocation(nil),
			"it skips heartbeat and adds allocation with matching events"},
		{[]*nomadApi.Events{&stopAllocationEvents},
			[]*nomadApi.Allocation{newStartedAllocation},
			[]*nomadApi.Allocation{newStoppedAllocation},
			"it adds and deletes the allocation"},
		{[]*nomadApi.Events{&startAllocationEvents, &startAllocationEvents},
			[]*nomadApi.Allocation{newStartedAllocation, newStartedAllocation},
			[]*nomadApi.Allocation(nil),
			"it handles multiple events"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertWatchAllocation(t, c.streamedEvents,
				c.expectedNewAllocations, c.expectedDeletedAllocations)
		})
	}
}

func TestHandleAllocationEventBuffersPendingAllocation(t *testing.T) {
	newPendingAllocation := createRecentAllocation(structs.AllocClientStatusPending, structs.AllocDesiredStatusRun)
	newPendingEvent := eventForAllocation(t, newPendingAllocation)

	pendingMap := make(map[string]time.Time)
	err := handleAllocationEvent(time.Now().UnixNano(), pendingMap, &newPendingEvent, noopAllocationProcessoring)
	require.NoError(t, err)

	_, ok := pendingMap[newPendingAllocation.ID]
	assert.True(t, ok)
}

func TestAPIClient_WatchAllocationsReturnsErrorWhenAllocationStreamCannotBeRetrieved(t *testing.T) {
	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", mock.Anything).Return(nil, tests.ErrDefault)
	apiClient := &APIClient{apiMock, map[string]chan error{}, false}

	err := apiClient.WatchEventStream(context.Background(), noopAllocationProcessoring)
	assert.ErrorIs(t, err, tests.ErrDefault)
}

func TestAPIClient_WatchAllocationsReturnsErrorWhenAllocationCannotBeRetrievedWithoutReceivingFurtherEvents(
	t *testing.T) {
	event := nomadApi.Event{
		Type:  structs.TypeAllocationUpdated,
		Topic: nomadApi.TopicAllocation,
		// This should fail decoding, as Allocation.ID is expected to be a string, not int
		Payload: map[string]interface{}{"Allocation": map[string]interface{}{"ID": 1}},
	}
	_, err := event.Allocation()
	require.Error(t, err)

	events := []*nomadApi.Events{{Events: []nomadApi.Event{event}}, {}}
	eventsProcessed, err := runAllocationWatching(t, events, noopAllocationProcessoring)
	assert.Error(t, err)
	assert.Equal(t, 1, eventsProcessed)
}

func assertWatchAllocation(t *testing.T, events []*nomadApi.Events,
	expectedNewAllocations, expectedDeletedAllocations []*nomadApi.Allocation) {
	t.Helper()
	var newAllocations []*nomadApi.Allocation
	var deletedAllocations []*nomadApi.Allocation
	callbacks := &AllocationProcessoring{
		OnNew: func(alloc *nomadApi.Allocation, _ time.Duration) {
			newAllocations = append(newAllocations, alloc)
		},
		OnDeleted: func(alloc *nomadApi.Allocation) {
			deletedAllocations = append(deletedAllocations, alloc)
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
func runAllocationWatching(t *testing.T, events []*nomadApi.Events, callbacks *AllocationProcessoring) (
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
func asynchronouslyWatchAllocations(stream chan *nomadApi.Events, callbacks *AllocationProcessoring) chan error {
	ctx := context.Background()
	// We can only get a read-only channel once we return it from a function.
	readOnlyStream := func() <-chan *nomadApi.Events { return stream }()

	apiMock := &apiQuerierMock{}
	apiMock.On("EventStream", ctx).Return(readOnlyStream, nil)
	apiClient := &APIClient{apiMock, map[string]chan error{}, false}

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
		ID:            tests.DefaultRunnerID,
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
	suite.Suite
	allocationID     string
	ctx              context.Context
	testCommand      string
	testCommandArray []string
	expectedStdout   string
	expectedStderr   string
	apiMock          *apiQuerierMock
	nomadAPIClient   APIClient
}

func (s *ExecuteCommandTestSuite) SetupTest() {
	s.allocationID = "test-allocation-id"
	s.ctx = context.Background()
	s.testCommand = "echo 'do nothing'"
	s.testCommandArray = []string{"sh", "-c", s.testCommand}
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
	var calledStdoutCommand, calledStderrCommand []string

	// mock regular call
	s.mockExecute(s.testCommandArray, commandExitCode, nil, func(args mock.Arguments) {
		var ok bool
		calledStdoutCommand, ok = args.Get(2).([]string)
		s.Require().True(ok)
		writer, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := writer.Write([]byte(s.expectedStdout))
		s.Require().NoError(err)
	})
	// mock stderr call
	s.mockExecute(mock.AnythingOfType("[]string"), stderrExitCode, nil, func(args mock.Arguments) {
		var ok bool
		calledStderrCommand, ok = args.Get(2).([]string)
		s.Require().True(ok)
		writer, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := writer.Write([]byte(s.expectedStderr))
		s.Require().NoError(err)
	})

	exitCode, err := s.nomadAPIClient.
		ExecuteCommand(s.allocationID, s.ctx, s.testCommandArray, withTTY, nullio.Reader{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 2)
	s.Equal(commandExitCode, exitCode)

	s.Run("should wrap command in stderr wrapper", func() {
		s.Require().NotNil(calledStdoutCommand)
		stderrWrapperCommand := fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoFormat, s.testCommand, stderrFifoFormat)
		stdoutFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrWrapperCommand), "%d", "\\d*")
		s.Regexp(stdoutFifoRegexp, calledStdoutCommand[len(calledStdoutCommand)-1])
	})

	s.Run("should call correct stderr command", func() {
		s.Require().NotNil(calledStderrCommand)
		stderrFifoCommand := fmt.Sprintf(stderrFifoCommandFormat, stderrFifoFormat, stderrFifoFormat, stderrFifoFormat)
		stderrFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrFifoCommand), "%d", "\\d*")
		s.Regexp(stderrFifoRegexp, calledStderrCommand[len(calledStderrCommand)-1])
	})

	s.Run("should return correct output", func() {
		s.Equal(s.expectedStdout, stdout.String())
		s.Equal(s.expectedStderr, stderr.String())
	})
}

func (s *ExecuteCommandTestSuite) TestWithSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = true
	s.mockExecute(s.testCommandArray, 1, tests.ErrDefault, func(args mock.Arguments) {})
	s.mockExecute(mock.AnythingOfType("[]string"), 1, nil, func(args mock.Arguments) {})
	_, err := s.nomadAPIClient.
		ExecuteCommand(s.allocationID, s.ctx, s.testCommandArray, withTTY, nullio.Reader{}, io.Discard, io.Discard)
	s.Equal(tests.ErrDefault, err)
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderr() {
	config.Config.Server.InteractiveStderr = false
	var stdout, stderr bytes.Buffer
	commandExitCode := 42

	// mock regular call
	s.mockExecute(s.testCommandArray, commandExitCode, nil, func(args mock.Arguments) {
		stdout, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := stdout.Write([]byte(s.expectedStdout))
		s.Require().NoError(err)
		stderr, ok := args.Get(6).(io.Writer)
		s.Require().True(ok)
		_, err = stderr.Write([]byte(s.expectedStderr))
		s.Require().NoError(err)
	})

	exitCode, err := s.nomadAPIClient.
		ExecuteCommand(s.allocationID, s.ctx, s.testCommandArray, withTTY, nullio.Reader{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 1)
	s.Equal(commandExitCode, exitCode)
	s.Equal(s.expectedStdout, stdout.String())
	s.Equal(s.expectedStderr, stderr.String())
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = false
	s.mockExecute(s.testCommandArray, 1, tests.ErrDefault, func(args mock.Arguments) {})
	_, err := s.nomadAPIClient.
		ExecuteCommand(s.allocationID, s.ctx, s.testCommandArray, withTTY, nullio.Reader{}, io.Discard, io.Discard)
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *ExecuteCommandTestSuite) mockExecute(command interface{}, exitCode int,
	err error, runFunc func(arguments mock.Arguments)) {
	s.apiMock.On("Execute", s.allocationID, s.ctx, command, withTTY,
		mock.Anything, mock.Anything, mock.Anything).
		Run(runFunc).
		Return(exitCode, err)
}

func TestAPIClient_LoadRunnerPortMappings(t *testing.T) {
	apiMock := &apiQuerierMock{}
	mockedCall := apiMock.On("allocation", tests.DefaultRunnerID)
	nomadAPIClient := APIClient{apiQuerier: apiMock}

	t.Run("should return error when API query fails", func(t *testing.T) {
		mockedCall.Return(nil, tests.ErrDefault)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		assert.Nil(t, portMappings)
		assert.ErrorIs(t, err, tests.ErrDefault)
	})

	t.Run("should return error when AllocatedResources is nil", func(t *testing.T) {
		mockedCall.Return(&nomadApi.Allocation{AllocatedResources: nil}, nil)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		assert.ErrorIs(t, err, ErrorNoAllocatedResourcesFound)
		assert.Nil(t, portMappings)
	})

	t.Run("should correctly return ports", func(t *testing.T) {
		allocation := &nomadApi.Allocation{
			AllocatedResources: &nomadApi.AllocatedResources{
				Shared: nomadApi.AllocatedSharedResources{Ports: tests.DefaultPortMappings},
			},
		}
		mockedCall.Return(allocation, nil)
		portMappings, err := nomadAPIClient.LoadRunnerPortMappings(tests.DefaultRunnerID)
		assert.NoError(t, err)
		assert.Equal(t, tests.DefaultPortMappings, portMappings)
	})
}
