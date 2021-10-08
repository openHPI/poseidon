package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
)

type CreateOrUpdateTestSuite struct {
	suite.Suite
	runnerManagerMock runner.ManagerMock
	apiMock           nomad.ExecutorAPIMock
	request           dto.ExecutionEnvironmentRequest
	manager           *NomadEnvironmentManager
	environmentID     dto.EnvironmentID
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
		runnerManager:          &s.runnerManagerMock,
		api:                    &s.apiMock,
		templateEnvironmentHCL: templateEnvironmentJobHCL,
	}

	s.environmentID = dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger)
}

func (s *CreateOrUpdateTestSuite) TestReturnsErrorIfCreatesOrUpdateEnvironmentReturnsError() {
	s.apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", tests.ErrDefault)
	s.runnerManagerMock.On("GetEnvironment", mock.AnythingOfType("dto.EnvironmentID")).Return(nil, false)
	s.runnerManagerMock.On("SetEnvironment", mock.AnythingOfType("*environment.NomadEnvironment")).Return(true)
	_, err := s.manager.CreateOrUpdate(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), s.request)
	s.ErrorIs(err, tests.ErrDefault)
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
		_, err := NewNomadEnvironment(templateJobHCL)
		require.NoError(t, err)
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.Equal(t, templateJobHCL, m.templateEnvironmentHCL)
	})

	t.Run("returns error if template file is invalid", func(t *testing.T) {
		templateJobHCL := "invalid hcl file"
		f := createTempFile(t, templateJobHCL)
		defer os.Remove(f.Name())

		m, err := NewNomadEnvironmentManager(runnerManagerMock, executorAPIMock, f.Name())
		require.NoError(t, err)
		_, err = NewNomadEnvironment(m.templateEnvironmentHCL)
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
