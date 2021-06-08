package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"math"
	"strconv"
	"testing"
)

type CreateOrUpdateTestSuite struct {
	suite.Suite
	runnerManagerMock        runner.ManagerMock
	apiMock                  nomad.ExecutorAPIMock
	registerNomadJobMockCall *mock.Call
	request                  dto.ExecutionEnvironmentRequest
	manager                  *NomadEnvironmentManager
}

func TestCreateOrUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(CreateOrUpdateTestSuite))
}

func (s *CreateOrUpdateTestSuite) SetupTest() {
	s.runnerManagerMock = runner.ManagerMock{}
	s.runnerManagerMock.On("registerEnvironment", mock.Anything, mock.Anything).Return(nil)

	s.apiMock = nomad.ExecutorAPIMock{}
	s.request = dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 10,
		CPULimit:           20,
		MemoryLimit:        30,
		Image:              "my-image",
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	s.registerNomadJobMockCall = s.apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("eval-id", nil)
	s.apiMock.On("MonitorEvaluation", mock.AnythingOfType("string"), mock.AnythingOfType("*context.emptyCtx")).Return(nil)

	s.manager = &NomadEnvironmentManager{
		runnerManager: &s.runnerManagerMock,
		api:           &s.apiMock,
	}
}

func (s *CreateOrUpdateTestSuite) mockEnvironmentExists(exists bool) {
	s.runnerManagerMock.On("EnvironmentExists", mock.AnythingOfType("EnvironmentID")).Return(exists)
}

func (s *CreateOrUpdateTestSuite) mockCreateOrUpdateEnvironment(exists bool) *mock.Call {
	return s.runnerManagerMock.On("CreateOrUpdateEnvironment",
		mock.AnythingOfType("EnvironmentID"), mock.AnythingOfType("uint")).
		Return(!exists, nil)
}

func (s *CreateOrUpdateTestSuite) createJobForRequest() *nomadApi.Job {
	return createJob(s.manager.defaultJob, nomad.DefaultJobID(tests.DefaultEnvironmentIdAsString),
		s.request.PrewarmingPoolSize, s.request.CPULimit, s.request.MemoryLimit,
		s.request.Image, s.request.NetworkAccess, s.request.ExposedPorts)
}

func (s *CreateOrUpdateTestSuite) TestFailsOnInvalidID() {
	_, err := s.manager.CreateOrUpdate("invalid-id", s.request)
	s.Error(err)
}

func (s *CreateOrUpdateTestSuite) TestFailsOnTooLargeID() {
	tooLargeIntStr := strconv.Itoa(math.MaxInt64) + "0"
	_, err := s.manager.CreateOrUpdate(tooLargeIntStr, s.request)
	s.Error(err)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsRegistersCorrectJob() {
	s.mockCreateOrUpdateEnvironment(true)
	expectedJob := s.createJobForRequest()

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.NoError(err)
	s.False(created)
	s.apiMock.AssertCalled(s.T(), "RegisterNomadJob", expectedJob)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsOccurredErrorIsPassed() {
	s.mockCreateOrUpdateEnvironment(true)

	s.registerNomadJobMockCall.Return("", tests.ErrDefault)
	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.False(created)
	s.Equal(tests.ErrDefault, err)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsReturnsFalse() {
	s.mockCreateOrUpdateEnvironment(true)

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.NoError(err)
	s.False(created)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistRegistersCorrectJob() {
	s.mockCreateOrUpdateEnvironment(false)

	expectedJob := s.createJobForRequest()

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.NoError(err)
	s.True(created)
	s.apiMock.AssertCalled(s.T(), "RegisterNomadJob", expectedJob)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistRegistersCorrectEnvironment() {
	s.mockCreateOrUpdateEnvironment(false)

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.True(created)
	s.NoError(err)
	s.runnerManagerMock.AssertCalled(s.T(), "CreateOrUpdateEnvironment",
		runner.EnvironmentID(tests.DefaultEnvironmentIdAsInteger), s.request.PrewarmingPoolSize)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistOccurredErrorIsPassedAndNoEnvironmentRegistered() {
	s.mockCreateOrUpdateEnvironment(false)

	s.registerNomadJobMockCall.Return("", tests.ErrDefault)
	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsString, s.request)
	s.False(created)
	s.Equal(tests.DefaultError, err)
}
