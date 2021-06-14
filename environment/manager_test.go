package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
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
	runnerManagerMock runner.ManagerMock
	apiMock           nomad.ExecutorAPIMock
	request           dto.ExecutionEnvironmentRequest
	manager           *NomadEnvironmentManager
	environmentID     runner.EnvironmentID
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

	s.manager = &NomadEnvironmentManager{
		runnerManager: &s.runnerManagerMock,
		api:           &s.apiMock,
	}

	s.environmentID = runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger)
}

func (s *CreateOrUpdateTestSuite) mockRegisterTemplateJob(job *nomadApi.Job, err error) {
	s.apiMock.On("RegisterTemplateJob",
		mock.AnythingOfType("*api.Job"), mock.AnythingOfType("string"),
		mock.AnythingOfType("uint"), mock.AnythingOfType("uint"), mock.AnythingOfType("uint"),
		mock.AnythingOfType("string"), mock.AnythingOfType("bool"), mock.AnythingOfType("[]uint16")).
		Return(job, err)
}

func (s *CreateOrUpdateTestSuite) mockCreateOrUpdateEnvironment(created bool, err error) {
	s.runnerManagerMock.On("CreateOrUpdateEnvironment", mock.AnythingOfType("EnvironmentID"),
		mock.AnythingOfType("uint"), mock.AnythingOfType("*api.Job"), mock.AnythingOfType("bool")).
		Return(created, err)
}

func (s *CreateOrUpdateTestSuite) createJobForRequest() *nomadApi.Job {
	return nomad.CreateTemplateJob(&s.manager.templateEnvironmentJob,
		runner.TemplateJobID(tests.DefaultEnvironmentIDAsInteger),
		s.request.PrewarmingPoolSize, s.request.CPULimit, s.request.MemoryLimit,
		s.request.Image, s.request.NetworkAccess, s.request.ExposedPorts)
}

func (s *CreateOrUpdateTestSuite) TestRegistersCorrectTemplateJob() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(true, nil)

	_, err := s.manager.CreateOrUpdate(s.environmentID, s.request)
	s.NoError(err)

	s.apiMock.AssertCalled(s.T(), "RegisterTemplateJob",
		&s.manager.templateEnvironmentJob, runner.TemplateJobID(s.environmentID),
		s.request.PrewarmingPoolSize, s.request.CPULimit, s.request.MemoryLimit,
		s.request.Image, s.request.NetworkAccess, s.request.ExposedPorts)
}

func (s *CreateOrUpdateTestSuite) TestReturnsErrorWhenRegisterTemplateJobReturnsError() {
	s.mockRegisterTemplateJob(nil, tests.ErrDefault)

	created, err := s.manager.CreateOrUpdate(s.environmentID, s.request)
	s.Equal(tests.ErrDefault, err)
	s.False(created)
}

func (s *CreateOrUpdateTestSuite) TestCreatesOrUpdatesCorrectEnvironment() {
	templateJobID := tests.DefaultJobID
	templateJob := &nomadApi.Job{ID: &templateJobID}
	s.mockRegisterTemplateJob(templateJob, nil)
	s.mockCreateOrUpdateEnvironment(true, nil)

	_, err := s.manager.CreateOrUpdate(s.environmentID, s.request)
	s.NoError(err)
	s.runnerManagerMock.AssertCalled(s.T(), "CreateOrUpdateEnvironment",
		s.environmentID, s.request.PrewarmingPoolSize, templateJob, true)
}

func (s *CreateOrUpdateTestSuite) TestReturnsErrorIfCreatesOrUpdateEnvironmentReturnsError() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(false, tests.ErrDefault)
	_, err := s.manager.CreateOrUpdate(runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.Equal(tests.ErrDefault, err)
}

func (s *CreateOrUpdateTestSuite) TestReturnsTrueIfCreatesOrUpdateEnvironmentReturnsTrue() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(true, nil)
	created, _ := s.manager.CreateOrUpdate(runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.True(created)
}

func (s *CreateOrUpdateTestSuite) TestReturnsFalseIfCreatesOrUpdateEnvironmentReturnsFalse() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(false, nil)
	created, _ := s.manager.CreateOrUpdate(runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.False(created)
}

func TestParseJob(t *testing.T) {
	exited := false
	logger, hook := test.NewNullLogger()
	logger.ExitFunc = func(i int) {
		exited = true
	}

	log = logger.WithField("pkg", "nomad")

	t.Run("parses the given default job", func(t *testing.T) {
		job := parseJob(templateEnvironmentJobHCL)
		assert.False(t, exited)
		assert.NotNil(t, job)
	})

	t.Run("fatals when given wrong job", func(t *testing.T) {
		job := parseJob("")
		assert.True(t, exited)
		assert.Nil(t, job)
		assert.Equal(t, logrus.FatalLevel, hook.LastEntry().Level)
	})
}
