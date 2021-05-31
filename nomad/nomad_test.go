package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"net/url"
	"testing"
	"time"
)

func TestLoadRunnersTestSuite(t *testing.T) {
	suite.Run(t, new(LoadRunnersTestSuite))
}

type LoadRunnersTestSuite struct {
	suite.Suite
	jobId                  string
	mock                   *apiQuerierMock
	nomadApiClient         ApiClient
	availableRunner        *nomadApi.AllocationListStub
	anotherAvailableRunner *nomadApi.AllocationListStub
	stoppedRunner          *nomadApi.AllocationListStub
	stoppingRunner         *nomadApi.AllocationListStub
}

func (suite *LoadRunnersTestSuite) SetupTest() {
	suite.jobId = "1d-0f-v3ry-sp3c14l-j0b"

	suite.mock = &apiQuerierMock{}
	suite.nomadApiClient = ApiClient{apiQuerier: suite.mock}

	suite.availableRunner = &nomadApi.AllocationListStub{
		ID:            "s0m3-r4nd0m-1d",
		ClientStatus:  nomadApi.AllocClientStatusRunning,
		DesiredStatus: nomadApi.AllocDesiredStatusRun,
	}

	suite.anotherAvailableRunner = &nomadApi.AllocationListStub{
		ID:            "s0m3-s1m1l4r-1d",
		ClientStatus:  nomadApi.AllocClientStatusRunning,
		DesiredStatus: nomadApi.AllocDesiredStatusRun,
	}

	suite.stoppedRunner = &nomadApi.AllocationListStub{
		ID:            "4n0th3r-1d",
		ClientStatus:  nomadApi.AllocClientStatusComplete,
		DesiredStatus: nomadApi.AllocDesiredStatusRun,
	}

	suite.stoppingRunner = &nomadApi.AllocationListStub{
		ID:            "th1rd-1d",
		ClientStatus:  nomadApi.AllocClientStatusRunning,
		DesiredStatus: nomadApi.AllocDesiredStatusStop,
	}
}

func (suite *LoadRunnersTestSuite) TestErrorOfUnderlyingApiCallIsPropagated() {
	errorString := "api errored"
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return(nil, errors.New(errorString))

	returnedIds, err := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.Nil(returnedIds)
	suite.Error(err)
}

func (suite *LoadRunnersTestSuite) TestThrowsNoErrorWhenUnderlyingApiCallDoesNot() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{}, nil)

	_, err := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.NoError(err)
}

func (suite *LoadRunnersTestSuite) TestAvailableRunnerIsReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.availableRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.Len(returnedIds, 1)
	suite.Equal(suite.availableRunner.ID, returnedIds[0])
}

func (suite *LoadRunnersTestSuite) TestStoppedRunnerIsNotReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.stoppedRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.Empty(returnedIds)
}

func (suite *LoadRunnersTestSuite) TestStoppingRunnerIsNotReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.stoppingRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.Empty(returnedIds)
}

func (suite *LoadRunnersTestSuite) TestReturnsAllAvailableRunners() {
	runnersList := []*nomadApi.AllocationListStub{
		suite.availableRunner,
		suite.anotherAvailableRunner,
		suite.stoppedRunner,
		suite.stoppingRunner,
	}
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return(runnersList, nil)

	returnedIds, _ := suite.nomadApiClient.LoadRunners(suite.jobId)
	suite.Len(returnedIds, 2)
	suite.Contains(returnedIds, suite.availableRunner.ID)
	suite.Contains(returnedIds, suite.anotherAvailableRunner.ID)
}

var (
	TestURL = url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:4646",
	}
)

const TestNamespace = "unit-tests"

func TestApiClient_init(t *testing.T) {
	client := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := client.init(&TestURL, TestNamespace)
	require.Nil(t, err)
}

func TestApiClientCanNotBeInitializedWithInvalidUrl(t *testing.T) {
	client := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := client.init(&url.URL{
		Scheme: "http",
		Host:   "http://127.0.0.1:4646",
	}, TestNamespace)
	assert.NotNil(t, err)
}

func TestNewExecutorApiCanBeCreatedWithoutError(t *testing.T) {
	expectedClient := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := expectedClient.init(&TestURL, TestNamespace)
	require.Nil(t, err)

	_, err = NewExecutorApi(&TestURL, TestNamespace)
	require.Nil(t, err)
}

// asynchronouslyMonitorEvaluation creates an ApiClient with mocked Nomad API and
// runs the MonitorEvaluation method in a goroutine. The mock returns a read-only
// version of the given stream to simulate an event stream gotten from the real
// Nomad API.
func asynchronouslyMonitorEvaluation(stream chan *nomadApi.Events) chan error {
	ctx := context.Background()
	// We can only get a read-only channel once we return it from a function.
	readOnlyStream := func() <-chan *nomadApi.Events { return stream }()

	apiMock := &apiQuerierMock{}
	apiMock.On("EvaluationStream", mock.AnythingOfType("string"), ctx).Return(readOnlyStream, nil)
	apiClient := &ApiClient{apiMock}

	errChan := make(chan error)
	go func() {
		errChan <- apiClient.MonitorEvaluation("id", ctx)
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
	expectedErr := errors.New("test error")
	apiMock.On("EvaluationStream", mock.AnythingOfType("string"), mock.AnythingOfType("*context.emptyCtx")).
		Return(nil, expectedErr)
	apiClient := &ApiClient{apiMock}
	err := apiClient.MonitorEvaluation("id", context.Background())
	assert.Equal(t, expectedErr, err)
}

type eventPayload struct {
	Evaluation *nomadApi.Evaluation
}

// eventForEvaluation takes an evaluation and creates an Event with the given evaluation
// as its payload. Nomad uses the mapstructure library to decode the payload, which we
// simply reverse here.
func eventForEvaluation(t *testing.T, eval nomadApi.Evaluation) nomadApi.Event {
	payload := make(map[string]interface{})

	err := mapstructure.Decode(eventPayload{&eval}, &payload)
	if err != nil {
		t.Fatalf("Couldn't encode evaluation %v", eval)
		return nomadApi.Event{}
	}
	event := nomadApi.Event{Topic: nomadApi.TopicEvaluation, Payload: payload}
	return event
}

// runEvaluationMonitoring simulates events streamed from the Nomad event stream
// to the MonitorEvaluation method. It starts the MonitorEvaluation function as a goroutine
// and sequentially transfers the events from the given array to a channel simulating the stream.
func runEvaluationMonitoring(t *testing.T, events []*nomadApi.Events) (eventsProcessed int, err error) {
	stream := make(chan *nomadApi.Events)
	errChan := asynchronouslyMonitorEvaluation(stream)

	var e *nomadApi.Events
	for eventsProcessed, e = range events {
		select {
		case err = <-errChan:
			return
		case stream <- e:
		}
	}
	// wait for error after streaming final event
	select {
	case err = <-errChan:
	case <-time.After(time.Millisecond * 10):
		t.Fatal("MonitorEvaluation didn't finish as expected")
	}
	// Increment once as range starts at 0
	eventsProcessed++
	return
}

func TestApiClient_MonitorEvaluationWithSuccessfulEvent(t *testing.T) {
	eval := nomadApi.Evaluation{Status: structs.EvalStatusComplete}
	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}

	// make sure that the tested function can complete
	require.Nil(t, checkEvaluation(&eval))

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(t, pendingEval), eventForEvaluation(t, eval),
	}}

	var cases = []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		name                    string
	}{
		{[]*nomadApi.Events{&events}, 1,
			"it completes with successful event"},
		{[]*nomadApi.Events{&events, &events}, 1,
			"it completes at first successful event"},
		{[]*nomadApi.Events{{}, &events}, 2,
			"it skips heartbeat and completes"},
		{[]*nomadApi.Events{&pendingEvaluationEvents, &events}, 2,
			"it skips pending evaluation and completes"},
		{[]*nomadApi.Events{&multipleEventsWithPending}, 1,
			"it handles multiple events per received event"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eventsProcessed, err := runEvaluationMonitoring(t, c.streamedEvents)
			assert.Nil(t, err)
			assert.Equal(t, c.expectedEventsProcessed, eventsProcessed)
		})
	}
}

func TestApiClient_MonitorEvaluationWithFailingEvent(t *testing.T) {
	eval := nomadApi.Evaluation{Status: structs.EvalStatusFailed}
	evalErr := checkEvaluation(&eval)
	require.NotNil(t, evalErr)

	pendingEval := nomadApi.Evaluation{Status: structs.EvalStatusPending}
	eventsErr := errors.New("my events error")

	events := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, eval)}}
	pendingEvaluationEvents := nomadApi.Events{Events: []nomadApi.Event{eventForEvaluation(t, pendingEval)}}
	multipleEventsWithPending := nomadApi.Events{Events: []nomadApi.Event{
		eventForEvaluation(t, pendingEval), eventForEvaluation(t, eval),
	}}
	eventsWithErr := nomadApi.Events{Err: eventsErr, Events: []nomadApi.Event{{}}}

	var cases = []struct {
		streamedEvents          []*nomadApi.Events
		expectedEventsProcessed int
		expectedError           error
		name                    string
	}{
		{[]*nomadApi.Events{&events}, 1, evalErr,
			"it fails with failing event"},
		{[]*nomadApi.Events{&events, &events}, 1, evalErr,
			"it fails at first failing event"},
		{[]*nomadApi.Events{{}, &events}, 2, evalErr,
			"it skips heartbeat and fail"},
		{[]*nomadApi.Events{&pendingEvaluationEvents, &events}, 2, evalErr,
			"it skips pending evaluation and fail"},
		{[]*nomadApi.Events{&multipleEventsWithPending}, 1, evalErr,
			"it handles multiple events per received event and fails"},
		{[]*nomadApi.Events{&eventsWithErr}, 1, eventsErr,
			"it fails with event error when event has error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eventsProcessed, err := runEvaluationMonitoring(t, c.streamedEvents)
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
	eventsProcessed, err := runEvaluationMonitoring(t, []*nomadApi.Events{{Events: []nomadApi.Event{event}}})
	assert.Equal(t, err, err)
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
		for _, status := range []string{structs.EvalStatusFailed, structs.EvalStatusCancelled, structs.EvalStatusBlocked, structs.EvalStatusPending} {
			evaluation.Status = status
			err := checkEvaluation(&evaluation)
			require.NotNil(t, err)
			assert.Contains(t, err.Error(), status, "error should contain the evaluation status")
		}
	})
}
