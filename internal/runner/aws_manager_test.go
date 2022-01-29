package runner

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestAWSRunnerManager_EnvironmentAccessor(t *testing.T) {
	m := NewAWSRunnerManager()

	environments := m.ListEnvironments()
	assert.Empty(t, environments)

	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	m.StoreEnvironment(environment)

	environments = m.ListEnvironments()
	assert.Len(t, environments, 1)
	assert.Equal(t, environments[0].ID(), dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))

	e, ok := m.GetEnvironment(tests.DefaultEnvironmentIDAsInteger)
	assert.True(t, ok)
	assert.Equal(t, environment, e)

	_, ok = m.GetEnvironment(tests.AnotherEnvironmentIDAsInteger)
	assert.False(t, ok)
}

func TestAWSRunnerManager_Claim(t *testing.T) {
	m := NewAWSRunnerManager()
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	r, err := NewAWSFunctionWorkload(environment, nil)
	assert.NoError(t, err)
	environment.On("Sample").Return(r, true)
	m.StoreEnvironment(environment)

	t.Run("returns runner for AWS environment", func(t *testing.T) {
		r, err := m.Claim(tests.DefaultEnvironmentIDAsInteger, 60)
		assert.NoError(t, err)
		assert.NotNil(t, r)
	})

	t.Run("forwards request for non AWS environments", func(t *testing.T) {
		nextHandler := &ManagerMock{}
		nextHandler.On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
			Return(nil, nil)
		m.SetNextHandler(nextHandler)

		_, err := m.Claim(tests.AnotherEnvironmentIDAsInteger, 60)
		assert.Nil(t, err)
		nextHandler.AssertCalled(t, "Claim", dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger), 60)
	})
}

func TestAWSRunnerManager_Return(t *testing.T) {
	m := NewAWSRunnerManager()
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	m.StoreEnvironment(environment)
	r, err := NewAWSFunctionWorkload(environment, nil)
	assert.NoError(t, err)

	t.Run("removes usedRunner", func(t *testing.T) {
		m.usedRunners.Add(r)
		assert.Contains(t, m.usedRunners.List(), r)

		err := m.Return(r)
		assert.NoError(t, err)
		assert.NotContains(t, m.usedRunners.List(), r)
	})

	t.Run("calls nextHandler for non AWS runner", func(t *testing.T) {
		nextHandler := &ManagerMock{}
		nextHandler.On("Return", mock.AnythingOfType("*runner.NomadJob")).Return(nil)
		m.SetNextHandler(nextHandler)

		nonAWSRunner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
		err := m.Return(nonAWSRunner)
		assert.NoError(t, err)
		nextHandler.AssertCalled(t, "Return", nonAWSRunner)
	})
}
