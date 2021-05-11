package nomad

import (
	"errors"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"net/url"
	"testing"
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

var TestURL = url.URL{
	Scheme: "http",
	Host:   "127.0.0.1:4646",
}

func TestApiClient_init(t *testing.T) {
	client := &ApiClient{}
	defaultJob := parseJob(defaultJobHCL)
	err := client.init(&TestURL)
	require.Nil(t, err)
	assert.Equal(t, *defaultJob, client.defaultJob)
}

func TestApiClientCanNotBeInitializedWithInvalidUrl(t *testing.T) {
	client := &ApiClient{}
	err := client.init(&url.URL{
		Scheme: "http",
		Host:   "http://127.0.0.1:4646",
	})
	assert.NotNil(t, err)
}

func TestNewExecutorApiCanBeCreatedWithoutError(t *testing.T) {
	expectedClient := &ApiClient{}
	err := expectedClient.init(&TestURL)
	require.Nil(t, err)

	_, err = NewExecutorApi(&TestURL)
	require.Nil(t, err)
}
