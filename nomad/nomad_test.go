package nomad

import (
	"errors"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestLoadAvailableRunnersTestSuite(t *testing.T) {
	suite.Run(t, new(LoadAvailableRunnersTestSuite))
}

type LoadAvailableRunnersTestSuite struct {
	suite.Suite
	jobId                  string
	mock                   *apiQuerierMock
	nomadApiClient         ApiClient
	availableRunner        *nomadApi.AllocationListStub
	anotherAvailableRunner *nomadApi.AllocationListStub
	stoppedRunner          *nomadApi.AllocationListStub
	stoppingRunner         *nomadApi.AllocationListStub
}

func (suite *LoadAvailableRunnersTestSuite) SetupTest() {
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

func (suite *LoadAvailableRunnersTestSuite) TestErrorOfUnderlyingApiCallIsPropagated() {
	errorString := "api errored"
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return(nil, errors.New(errorString))

	returnedIds, err := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.Nil(returnedIds)
	suite.Error(err)
}

func (suite *LoadAvailableRunnersTestSuite) TestThrowsNoErrorWhenUnderlyingApiCallDoesNot() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{}, nil)

	_, err := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.NoError(err)
}

func (suite *LoadAvailableRunnersTestSuite) TestAvailableRunnerIsReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.availableRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.Len(returnedIds, 1)
	suite.Equal(suite.availableRunner.ID, returnedIds[0])
}

func (suite *LoadAvailableRunnersTestSuite) TestStoppedRunnerIsNotReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.stoppedRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.Empty(returnedIds)
}

func (suite *LoadAvailableRunnersTestSuite) TestStoppingRunnerIsNotReturned() {
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return([]*nomadApi.AllocationListStub{suite.stoppingRunner}, nil)

	returnedIds, _ := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.Empty(returnedIds)
}

func (suite *LoadAvailableRunnersTestSuite) TestReturnsAllAvailableRunners() {
	runnersList := []*nomadApi.AllocationListStub{
		suite.availableRunner,
		suite.anotherAvailableRunner,
		suite.stoppedRunner,
		suite.stoppingRunner,
	}
	suite.mock.On("loadRunners", mock.AnythingOfType("string")).
		Return(runnersList, nil)

	returnedIds, _ := suite.nomadApiClient.LoadAvailableRunners(suite.jobId)
	suite.Len(returnedIds, 2)
	suite.Contains(returnedIds, suite.availableRunner.ID)
	suite.Contains(returnedIds, suite.anotherAvailableRunner.ID)
}
