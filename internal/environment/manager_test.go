package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"os"
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
	s.ErrorIs(err, tests.ErrDefault)
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
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *CreateOrUpdateTestSuite) TestReturnsTrueIfCreatesOrUpdateEnvironmentReturnsTrue() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(true, nil)
	created, err := s.manager.CreateOrUpdate(runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.Require().NoError(err)
	s.True(created)
}

func (s *CreateOrUpdateTestSuite) TestReturnsFalseIfCreatesOrUpdateEnvironmentReturnsFalse() {
	s.mockRegisterTemplateJob(&nomadApi.Job{}, nil)
	s.mockCreateOrUpdateEnvironment(false, nil)
	created, err := s.manager.CreateOrUpdate(runner.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.Require().NoError(err)
	s.False(created)
}

func TestParseJob(t *testing.T) {
	t.Run("parses the given default job", func(t *testing.T) {
		job, err := parseJob(templateEnvironmentJobHCL)
		assert.NoError(t, err)
		assert.NotNil(t, job)
	})

	t.Run("returns error when given wrong job", func(t *testing.T) {
		job, err := parseJob("")
		assert.Error(t, err)
		assert.Nil(t, job)
	})
}

func TestNewNomadEnvironmentManager(t *testing.T) {
	executorAPIMock := &nomad.ExecutorAPIMock{}
	executorAPIMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)

	runnerManagerMock := &runner.ManagerMock{}
	runnerManagerMock.On("Load").Return()

	previousTemplateEnvironmentJobHCL := templateEnvironmentJobHCL

	t.Run("returns error if template file does not exist", func(t *testing.T) {
		_, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, "/non-existent/file")
		assert.Error(t, err)
	})

	t.Run("loads template environment job from file", func(t *testing.T) {
		templateJobHCL := "job \"test\" {}"
		expectedJob, err := parseJob(templateJobHCL)
		require.NoError(t, err)
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, templateJobHCL, templateEnvironmentJobHCL)
		assert.Equal(t, *expectedJob, m.templateEnvironmentJob)
	})

	t.Run("returns error if template file is invalid", func(t *testing.T) {
		templateJobHCL := "invalid hcl file"
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		_, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		assert.Error(t, err)
	})

	templateEnvironmentJobHCL = previousTemplateEnvironmentJobHCL
}

func createTempFile(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "test")
	require.NoError(t, err)
	n, err := f.WriteString(content)
	require.NoError(t, err)
	require.Equal(t, len(content), n)

	return f
}
