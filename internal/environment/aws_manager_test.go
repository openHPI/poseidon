package environment

import (
	"context"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAWSEnvironmentManager_CreateOrUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	m := NewAWSEnvironmentManager(runnerManager)
	uniqueImage := "java11Exec"

	t.Run("can create default Java environment", func(t *testing.T) {
		config.Config.AWS.Functions = []string{uniqueImage}
		_, err := m.CreateOrUpdate(
			tests.AnotherEnvironmentIDAsInteger, dto.ExecutionEnvironmentRequest{Image: uniqueImage}, context.Background())
		assert.NoError(t, err)
	})

	t.Run("can retrieve added environment", func(t *testing.T) {
		environment, err := m.Get(tests.AnotherEnvironmentIDAsInteger, false)
		assert.NoError(t, err)
		assert.Equal(t, environment.Image(), uniqueImage)
	})

	t.Run("non-handleable requests are forwarded to the next manager", func(t *testing.T) {
		nextHandler := &ManagerHandlerMock{}
		nextHandler.On("CreateOrUpdate", mock.AnythingOfType("dto.EnvironmentID"),
			mock.AnythingOfType("dto.ExecutionEnvironmentRequest"), mock.Anything).Return(true, nil)
		m.SetNextHandler(nextHandler)

		request := dto.ExecutionEnvironmentRequest{}
		_, err := m.CreateOrUpdate(tests.DefaultEnvironmentIDAsInteger, request, context.Background())
		assert.NoError(t, err)
		nextHandler.AssertCalled(t, "CreateOrUpdate",
			dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), request, mock.Anything)
	})
}

func TestAWSEnvironmentManager_Get(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	m := NewAWSEnvironmentManager(runnerManager)

	t.Run("Calls next handler when not found", func(t *testing.T) {
		nextHandler := &ManagerHandlerMock{}
		nextHandler.On("Get", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("bool")).
			Return(nil, nil)
		m.SetNextHandler(nextHandler)

		_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		assert.NoError(t, err)
		nextHandler.AssertCalled(t, "Get", dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger), false)
	})

	t.Run("Returns error when not found", func(t *testing.T) {
		nextHandler := &AbstractManager{nil, nil}
		m.SetNextHandler(nextHandler)

		_, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		assert.ErrorIs(t, err, runner.ErrRunnerNotFound)
	})

	t.Run("Returns environment when it was added before", func(t *testing.T) {
		expectedEnvironment := NewAWSEnvironment(nil)
		expectedEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(expectedEnvironment)

		environment, err := m.Get(tests.DefaultEnvironmentIDAsInteger, false)
		assert.NoError(t, err)
		assert.Equal(t, expectedEnvironment, environment)
	})
}

func TestAWSEnvironmentManager_List(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerManager := runner.NewAWSRunnerManager(ctx)
	m := NewAWSEnvironmentManager(runnerManager)

	t.Run("also returns environments of the rest of the manager chain", func(t *testing.T) {
		nextHandler := &ManagerHandlerMock{}
		existingEnvironment := NewAWSEnvironment(nil)
		nextHandler.On("List", mock.AnythingOfType("bool")).
			Return([]runner.ExecutionEnvironment{existingEnvironment}, nil)
		m.SetNextHandler(nextHandler)

		environments, err := m.List(false)
		assert.NoError(t, err)
		require.Len(t, environments, 1)
		assert.Contains(t, environments, existingEnvironment)
	})
	m.SetNextHandler(nil)

	t.Run("Returns added environment", func(t *testing.T) {
		localEnvironment := NewAWSEnvironment(nil)
		localEnvironment.SetID(tests.DefaultEnvironmentIDAsInteger)
		runnerManager.StoreEnvironment(localEnvironment)

		environments, err := m.List(false)
		assert.NoError(t, err)
		assert.Len(t, environments, 1)
		assert.Contains(t, environments, localEnvironment)
	})
}
