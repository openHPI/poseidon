package runner

import (
	"testing"

	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestAWSRunnerManager_EnvironmentAccessor() {
	runnerManager := NewAWSRunnerManager(s.TestCtx)

	environments := runnerManager.ListEnvironments()
	s.Empty(environments)

	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	runnerManager.StoreEnvironment(environment)

	environments = runnerManager.ListEnvironments()
	s.Len(environments, 1)
	s.Equal(environments[0].ID(), dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))

	e, ok := runnerManager.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
	s.True(ok)
	s.Equal(environment, e)

	_, ok = runnerManager.GetEnvironment(tests.AnotherEnvironmentIDAsInteger)
	s.False(ok)
}

func (s *MainTestSuite) TestAWSRunnerManager_Claim() {
	runnerManager := NewAWSRunnerManager(s.TestCtx)
	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	runnerWorkload, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.NoError(err)
	environment.On("Sample").Return(runnerWorkload, true)
	runnerManager.StoreEnvironment(environment)

	s.Run("returns runner for AWS environment", func() {
		r, err := runnerManager.Claim(tests.DefaultEnvironmentIDAsInteger, 60)
		s.NoError(err)
		s.NotNil(r)
	})

	s.Run("forwards request for non-AWS environments", func() {
		nextHandler := &ManagerMock{}
		nextHandler.On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
			Return(nil, nil)
		runnerManager.SetNextHandler(nextHandler)

		_, err := runnerManager.Claim(tests.AnotherEnvironmentIDAsInteger, 60)
		s.NoError(err)
		nextHandler.AssertCalled(s.T(), "Claim", dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger), 60)
	})

	err = runnerWorkload.Destroy(nil)
	s.NoError(err)
}

func (s *MainTestSuite) TestAWSRunnerManager_Return() {
	runnerManager := NewAWSRunnerManager(s.TestCtx)
	environment := createBasicEnvironmentMock(defaultEnvironmentID)
	runnerManager.StoreEnvironment(environment)
	runnerWorkload, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.NoError(err)

	s.Run("removes usedRunner", func() {
		runnerManager.usedRunners.Add(runnerWorkload.ID(), runnerWorkload)
		s.Contains(runnerManager.usedRunners.List(), runnerWorkload)

		err := runnerManager.Return(runnerWorkload)
		s.NoError(err)
		s.NotContains(runnerManager.usedRunners.List(), runnerWorkload)
	})

	s.Run("calls nextHandler for non-AWS runner", func() {
		nextHandler := &ManagerMock{}
		nextHandler.On("Return", mock.AnythingOfType("*runner.NomadJob")).Return(nil)
		runnerManager.SetNextHandler(nextHandler)

		apiMock := &nomad.ExecutorAPIMock{}
		apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
		nonAWSRunner := NewNomadJob(tests.DefaultRunnerID, nil, apiMock, nil)
		err := runnerManager.Return(nonAWSRunner)
		s.NoError(err)
		nextHandler.AssertCalled(s.T(), "Return", nonAWSRunner)

		err = nonAWSRunner.Destroy(nil)
		s.NoError(err)
	})

	err = runnerWorkload.Destroy(nil)
	s.NoError(err)
}

func createBasicEnvironmentMock(id dto.EnvironmentID) *ExecutionEnvironmentMock {
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(id)
	environment.On("Image").Return("")
	environment.On("CPULimit").Return(uint(0))
	environment.On("MemoryLimit").Return(uint(0))
	environment.On("NetworkAccess").Return(false, nil)
	environment.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil, false)
	environment.On("ApplyPrewarmingPoolSize").Return(nil)
	environment.On("IdleRunnerCount").Return(uint(1)).Maybe()
	environment.On("PrewarmingPoolSize").Return(uint(1)).Maybe()
	return environment
}
