package runner

import (
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"testing"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestAWSRunnerManager_EnvironmentAccessor() {
	m := NewAWSRunnerManager(s.TestCtx)

	environments := m.ListEnvironments()
	s.Empty(environments)

	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	m.StoreEnvironment(environment)

	environments = m.ListEnvironments()
	s.Len(environments, 1)
	s.Equal(environments[0].ID(), dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))

	e, ok := m.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
	s.True(ok)
	s.Equal(environment, e)

	_, ok = m.GetEnvironment(tests.AnotherEnvironmentIDAsInteger)
	s.False(ok)
}

func (s *MainTestSuite) TestAWSRunnerManager_Claim() {
	m := NewAWSRunnerManager(s.TestCtx)
	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	r, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.NoError(err)
	environment.On("Sample").Return(r, true)
	m.StoreEnvironment(environment)

	s.Run("returns runner for AWS environment", func() {
		r, err := m.Claim(tests.DefaultEnvironmentIDAsInteger, 60)
		s.NoError(err)
		s.NotNil(r)
	})

	s.Run("forwards request for non-AWS environments", func() {
		nextHandler := &ManagerMock{}
		nextHandler.On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
			Return(nil, nil)
		m.SetNextHandler(nextHandler)

		_, err := m.Claim(tests.AnotherEnvironmentIDAsInteger, 60)
		s.Nil(err)
		nextHandler.AssertCalled(s.T(), "Claim", dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger), 60)
	})

	err = r.Destroy(nil)
	s.NoError(err)
}

func (s *MainTestSuite) TestAWSRunnerManager_Return() {
	m := NewAWSRunnerManager(s.TestCtx)
	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	m.StoreEnvironment(environment)
	r, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.NoError(err)

	s.Run("removes usedRunner", func() {
		m.usedRunners.Add(r.ID(), r)
		s.Contains(m.usedRunners.List(), r)

		err := m.Return(r)
		s.NoError(err)
		s.NotContains(m.usedRunners.List(), r)
	})

	s.Run("calls nextHandler for non-AWS runner", func() {
		nextHandler := &ManagerMock{}
		nextHandler.On("Return", mock.AnythingOfType("*runner.NomadJob")).Return(nil)
		m.SetNextHandler(nextHandler)

		apiMock := &nomad.ExecutorAPIMock{}
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		nonAWSRunner := NewNomadJob(tests.DefaultRunnerID, nil, apiMock, nil)
		err := m.Return(nonAWSRunner)
		s.NoError(err)
		nextHandler.AssertCalled(s.T(), "Return", nonAWSRunner)

		err = nonAWSRunner.Destroy(nil)
		s.NoError(err)
	})

	err = r.Destroy(nil)
	s.NoError(err)
}

func createBasicEnvironmentMock(id dto.EnvironmentID) *ExecutionEnvironmentMock {
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(id)
	environment.On("Image").Return("")
	environment.On("CPULimit").Return(uint(0))
	environment.On("MemoryLimit").Return(uint(0))
	environment.On("NetworkAccess").Return(false, nil)
	environment.On("DeleteRunner", mock.AnythingOfType("string")).Return(false)
	environment.On("ApplyPrewarmingPoolSize").Return(nil)
	return environment
}
