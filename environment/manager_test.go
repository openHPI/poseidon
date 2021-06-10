package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
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

func (s *CreateOrUpdateTestSuite) mockCreateOrUpdateEnvironment(exists bool) *mock.Call {
	return s.runnerManagerMock.On("CreateOrUpdateEnvironment",
		mock.AnythingOfType("EnvironmentID"), mock.AnythingOfType("uint"), mock.AnythingOfType("*api.Job")).
		Return(!exists, nil)
}

func (s *CreateOrUpdateTestSuite) createJobForRequest() *nomadApi.Job {
	return createTemplateJob(s.manager.defaultJob, tests.DefaultEnvironmentIDAsInteger,
		s.request.PrewarmingPoolSize, s.request.CPULimit, s.request.MemoryLimit,
		s.request.Image, s.request.NetworkAccess, s.request.ExposedPorts)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsRegistersCorrectJob() {
	s.mockCreateOrUpdateEnvironment(true)
	expectedJob := s.createJobForRequest()

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.NoError(err)
	s.False(created)
	s.apiMock.AssertCalled(s.T(), "RegisterNomadJob", expectedJob)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsOccurredErrorIsPassed() {
	s.mockCreateOrUpdateEnvironment(true)

	s.registerNomadJobMockCall.Return("", tests.ErrDefault)
	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.False(created)
	s.Equal(tests.ErrDefault, err)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentExistsReturnsFalse() {
	s.mockCreateOrUpdateEnvironment(true)

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.NoError(err)
	s.False(created)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistRegistersCorrectJob() {
	s.mockCreateOrUpdateEnvironment(false)

	expectedJob := s.createJobForRequest()

	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.NoError(err)
	s.True(created)
	s.apiMock.AssertCalled(s.T(), "RegisterNomadJob", expectedJob)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistRegistersCorrectEnvironment() {
	s.mockCreateOrUpdateEnvironment(false)

	expectedJob := s.createJobForRequest()
	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.True(created)
	s.NoError(err)
	s.runnerManagerMock.AssertCalled(s.T(), "CreateOrUpdateEnvironment",
		runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request.PrewarmingPoolSize, expectedJob)
}

func (s *CreateOrUpdateTestSuite) TestWhenEnvironmentDoesNotExistOccurredErrorIsPassedAndNoEnvironmentRegistered() {
	s.mockCreateOrUpdateEnvironment(false)

	s.registerNomadJobMockCall.Return("", tests.ErrDefault)
	created, err := s.manager.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, s.request)
	s.False(created)
	s.Equal(tests.ErrDefault, err)
	s.runnerManagerMock.AssertNotCalled(s.T(), "CreateOrUpdateEnvironment")
}
