package environment

import (
	"context"
	"testing"

	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/runner"
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

func (s *MainTestSuite) TestAWSEnvironmentManager_CreateOrUpdate() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	environmentManager := NewAWSEnvironmentManager(runnerManager)
	uniqueImage := "java11Exec"

	s.Run("can create default Java environment", func() {
		config.Config.AWS.Functions = []string{uniqueImage}
		_, err := environmentManager.CreateOrUpdate(
			context.Background(), tests.AnotherEnvironmentIDAsInteger, dto.ExecutionEnvironmentRequest{Image: uniqueImage})
		s.NoError(err)
	})

	s.Run("can retrieve added environment", func() {
		environment, err := environmentManager.Get(s.TestCtx, tests.AnotherEnvironmentIDAsInteger, false)
		s.NoError(err)
		s.Equal(environment.Image(), uniqueImage)
	})

	s.Run("non-handleable requests are forwarded to the next manager", func() {
		nextHandler := &ManagerHandlerMock{}
		nextHandler.On("CreateOrUpdate", mock.Anything, mock.AnythingOfType("dto.EnvironmentID"),
			mock.AnythingOfType("dto.ExecutionEnvironmentRequest")).Return(true, nil)
		environmentManager.SetNextHandler(nextHandler)

		request := dto.ExecutionEnvironmentRequest{}
		_, err := environmentManager.CreateOrUpdate(context.Background(), tests.DefaultEnvironmentIDAsInteger, request)
		s.NoError(err)
		nextHandler.AssertCalled(s.T(), "CreateOrUpdate", mock.Anything,
			dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), request)
	})
}

func (s *MainTestSuite) TestAWSEnvironmentManager_Get() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	environmentManager := NewAWSEnvironmentManager(runnerManager)

	s.Run("Calls next handler when not found", func() {
		nextHandler := &ManagerHandlerMock{}
		nextHandler.On("Get", mock.Anything, mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("bool")).
			Return(nil, nil)
		environmentManager.SetNextHandler(nextHandler)

		_, err := environmentManager.Get(s.TestCtx, tests.DefaultEnvironmentIDAsInteger, false)
		s.NoError(err)
		nextHandler.AssertCalled(s.T(), "Get", mock.Anything, dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), false)
	})

	s.Run("Returns error when not found", func() {
		nextHandler := &AbstractManager{nil, nil}
		environmentManager.SetNextHandler(nextHandler)

		_, err := environmentManager.Get(s.TestCtx, tests.DefaultEnvironmentIDAsInteger, false)
		s.ErrorIs(err, runner.ErrRunnerNotFound)
	})

	s.Run("Returns environment when it was added before", func() {
		expectedEnvironment := NewAWSEnvironment(nil)
		expectedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(expectedEnvironment)

		environment, err := environmentManager.Get(s.TestCtx, tests.DefaultEnvironmentIDAsInteger, false)
		s.Require().NoError(err)
		s.Equal(expectedEnvironment, environment)
	})
}

func (s *MainTestSuite) TestAWSEnvironmentManager_List() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	awsEnvironmentManager := NewAWSEnvironmentManager(runnerManager)

	s.Run("also returns environments of the rest of the manager chain", func() {
		nextHandler := &ManagerHandlerMock{}
		existingEnvironment := NewAWSEnvironment(nil)
		nextHandler.On("List", mock.Anything, mock.AnythingOfType("bool")).
			Return([]runner.ExecutionEnvironment{existingEnvironment}, nil)
		awsEnvironmentManager.SetNextHandler(nextHandler)

		environments, err := awsEnvironmentManager.List(s.TestCtx, false)
		s.NoError(err)
		s.Require().Len(environments, 1)
		s.Contains(environments, existingEnvironment)
	})
	awsEnvironmentManager.SetNextHandler(nil)

	s.Run("Returns added environment", func() {
		localEnvironment := NewAWSEnvironment(nil)
		localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(localEnvironment)

		environments, err := awsEnvironmentManager.List(s.TestCtx, false)
		s.NoError(err)
		s.Len(environments, 1)
		s.Contains(environments, localEnvironment)
	})
}
