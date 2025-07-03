package nomad

import (
	"testing"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

	returnedIDs, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Nil(returnedIDs)
	s.Equal(tests.ErrDefault, err)
}

func (s *LoadRunnersTestSuite) TestReturnsNoErrorWhenUnderlyingApiCallDoesNot() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{}, nil)

	_, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
}

func (s *LoadRunnersTestSuite) TestAvailableRunnerIsReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.availableRunner}, nil)

	returnedIDs, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIDs, 1)
	s.Equal(s.availableRunner.ID, returnedIDs[0])
}

func (s *LoadRunnersTestSuite) TestPendingRunnerIsReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.pendingRunner}, nil)

	returnedIDs, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIDs, 1)
	s.Equal(s.pendingRunner.ID, returnedIDs[0])
}

func (s *LoadRunnersTestSuite) TestDeadRunnerIsNotReturned() {
	s.mock.On("listJobs", mock.AnythingOfType("string")).
		Return([]*nomadApi.JobListStub{s.deadRunner}, nil)

	returnedIDs, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Empty(returnedIDs)
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

	returnedIDs, err := s.nomadAPIClient.LoadRunnerIDs(s.jobID)
	s.Require().NoError(err)
	s.Len(returnedIDs, 3)
	s.Contains(returnedIDs, s.availableRunner.ID)
	s.Contains(returnedIDs, s.anotherAvailableRunner.ID)
	s.Contains(returnedIDs, s.pendingRunner.ID)
	s.NotContains(returnedIDs, s.deadRunner.ID)
}

const (
	TestNamespace      = "unit-tests"
	TestNomadToken     = "n0m4d-t0k3n"
	TestDefaultAddress = "127.0.0.1"
	evaluationID       = "evaluation-id"
)

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
	s.Require().NoError(err)
}

func (s *MainTestSuite) TestApiClientCanNotBeInitializedWithInvalidUrl() {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(NomadTestConfig("http://" + TestDefaultAddress))
	s.Error(err)
}

func (s *MainTestSuite) TestNewExecutorApiCanBeCreatedWithoutError() {
	expectedClient := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := expectedClient.init(NomadTestConfig(TestDefaultAddress))
	s.Require().NoError(err)

	_, err = NewExecutorAPI(s.TestCtx, NomadTestConfig(TestDefaultAddress))
	s.Require().NoError(err)
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
		s.Require().ErrorIs(err, ErrNoAllocatedResourcesFound)
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
		s.Require().NoError(err)
		s.Equal(tests.DefaultPortMappings, portMappings)
	})
}
