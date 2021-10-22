package e2e

import (
	"encoding/json"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreateOrUpdateEnvironment(t *testing.T) {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIDAsString)

	t.Run("returns bad request with empty body", func(t *testing.T) {
		resp, err := helpers.HTTPPut(path, strings.NewReader(""))
		require.Nil(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		_ = resp.Body.Close()
	})

	request := dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 1,
		CPULimit:           100,
		MemoryLimit:        100,
		Image:              *testDockerImage,
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	t.Run("creates correct environment in Nomad", func(t *testing.T) {
		assertPutReturnsStatusAndZeroContent(t, path, request, http.StatusCreated)
		validateJob(t, request)
	})

	t.Run("updates limits in Nomad correctly", func(t *testing.T) {
		updateRequest := request
		updateRequest.CPULimit = 150
		updateRequest.MemoryLimit = 142

		assertPutReturnsStatusAndZeroContent(t, path, updateRequest, http.StatusNoContent)
		validateJob(t, updateRequest)
	})

	t.Run("adds network correctly", func(t *testing.T) {
		updateRequest := request
		updateRequest.NetworkAccess = true
		updateRequest.ExposedPorts = []uint16{42, 1337}

		assertPutReturnsStatusAndZeroContent(t, path, updateRequest, http.StatusNoContent)
		validateJob(t, updateRequest)
	})

	t.Run("removes network correctly", func(t *testing.T) {
		require.False(t, request.NetworkAccess)
		require.Nil(t, request.ExposedPorts)
		assertPutReturnsStatusAndZeroContent(t, path, request, http.StatusNoContent)
		validateJob(t, request)
	})

	deleteEnvironment(t, tests.AnotherEnvironmentIDAsString)
}

func TestListEnvironments(t *testing.T) {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath)

	t.Run("returns list with one element", func(t *testing.T) {
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		environmentsArray := assertEnvironmentArrayInResponse(t, response)
		assert.Equal(t, 1, len(environmentsArray))
	})

	t.Run("returns list including the default environment", func(t *testing.T) {
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)

		environmentsArray := assertEnvironmentArrayInResponse(t, response)
		require.Equal(t, 1, len(environmentsArray))

		assertEnvironment(t, environmentsArray[0], tests.DefaultEnvironmentIDAsInteger)
	})

	t.Run("Added environments can be retrieved without fetch", func(t *testing.T) {
		createEnvironment(t, tests.AnotherEnvironmentIDAsString)

		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)

		environmentsArray := assertEnvironmentArrayInResponse(t, response)
		require.Equal(t, 2, len(environmentsArray))
		foundIDs := parseIDsFromEnvironments(t, environmentsArray)
		assert.Contains(t, foundIDs, dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger))
	})
	deleteEnvironment(t, tests.AnotherEnvironmentIDAsString)

	t.Run("Added environments can be retrieved with fetch", func(t *testing.T) {
		// Add environment without Poseidon
		_, job := helpers.CreateTemplateJob()
		jobID := nomad.TemplateJobID(tests.AnotherEnvironmentIDAsInteger)
		job.ID = &jobID
		job.Name = &jobID
		_, _, err := nomadClient.Jobs().Register(job, nil)
		require.NoError(t, err)
		<-time.After(tests.ShortTimeout) // Nomad needs a bit to create the job

		// List without fetch should not include the added environment
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		environmentsArray := assertEnvironmentArrayInResponse(t, response)
		require.Equal(t, 1, len(environmentsArray))
		assertEnvironment(t, environmentsArray[0], tests.DefaultEnvironmentIDAsInteger)

		// List with fetch should include the added environment
		response, err = http.Get(path + "?fetch=true") //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		environmentsArray = assertEnvironmentArrayInResponse(t, response)
		require.Equal(t, 2, len(environmentsArray))
		foundIDs := parseIDsFromEnvironments(t, environmentsArray)
		assert.Contains(t, foundIDs, dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger))
	})
	deleteEnvironment(t, tests.AnotherEnvironmentIDAsString)
}

func TestGetEnvironment(t *testing.T) {
	t.Run("returns the default environment", func(t *testing.T) {
		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)

		environment := getEnvironmentFromResponse(t, response)
		assertEnvironment(t, environment, tests.DefaultEnvironmentIDAsInteger)
	})

	t.Run("Added environments can be retrieved without fetch", func(t *testing.T) {
		createEnvironment(t, tests.AnotherEnvironmentIDAsString)

		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIDAsString)
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)

		environment := getEnvironmentFromResponse(t, response)
		assertEnvironment(t, environment, tests.AnotherEnvironmentIDAsInteger)
	})
	deleteEnvironment(t, tests.AnotherEnvironmentIDAsString)

	t.Run("Added environments can be retrieved with fetch", func(t *testing.T) {
		// Add environment without Poseidon
		_, job := helpers.CreateTemplateJob()
		jobID := nomad.TemplateJobID(tests.AnotherEnvironmentIDAsInteger)
		job.ID = &jobID
		job.Name = &jobID
		_, _, err := nomadClient.Jobs().Register(job, nil)
		require.NoError(t, err)
		<-time.After(tests.ShortTimeout) // Nomad needs a bit to create the job

		// List without fetch should not include the added environment
		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIDAsString)
		response, err := http.Get(path) //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, response.StatusCode)

		// List with fetch should include the added environment
		response, err = http.Get(path + "?fetch=true") //nolint:gosec // because we build this path right above
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		environment := getEnvironmentFromResponse(t, response)
		assertEnvironment(t, environment, tests.AnotherEnvironmentIDAsInteger)
	})
	deleteEnvironment(t, tests.AnotherEnvironmentIDAsString)
}

func TestDeleteEnvironment(t *testing.T) {
	t.Run("Removes added environment", func(t *testing.T) {
		createEnvironment(t, tests.AnotherEnvironmentIDAsString)

		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIDAsString)
		response, err := helpers.HTTPDelete(path, nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)
	})

	t.Run("Removes Nomad Job", func(t *testing.T) {
		createEnvironment(t, tests.AnotherEnvironmentIDAsString)

		// Expect created Nomad job
		jobID := nomad.TemplateJobID(tests.AnotherEnvironmentIDAsInteger)
		job, _, err := nomadClient.Jobs().Info(jobID, nil)
		assert.NoError(t, err)
		assert.Equal(t, jobID, *job.ID)

		// Delete the job
		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIDAsString)
		response, err := helpers.HTTPDelete(path, nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)

		// Expect not to find the Nomad job
		_, _, err = nomadClient.Jobs().Info(jobID, nil)
		assert.Error(t, err)
	})
}

func parseIDsFromEnvironments(t *testing.T, environments []interface{}) (ids []dto.EnvironmentID) {
	t.Helper()
	for _, environment := range environments {
		id, _ := parseEnvironment(t, environment)
		ids = append(ids, id)
	}
	return ids
}

func assertEnvironment(t *testing.T, environment interface{}, expectedID dto.EnvironmentID) {
	t.Helper()
	id, defaultEnvironmentParams := parseEnvironment(t, environment)

	assert.Equal(t, expectedID, id)
	expectedKeys := []string{"prewarmingPoolSize", "cpuLimit", "memoryLimit", "image", "networkAccess", "exposedPorts"}
	for _, key := range expectedKeys {
		_, ok := defaultEnvironmentParams[key]
		assert.True(t, ok)
	}
}

func parseEnvironment(t *testing.T, environment interface{}) (id dto.EnvironmentID, params map[string]interface{}) {
	t.Helper()
	environmentParams, ok := environment.(map[string]interface{})
	require.True(t, ok)
	idInterface, ok := environmentParams["id"]
	require.True(t, ok)
	idFloat, ok := idInterface.(float64)
	require.True(t, ok)
	return dto.EnvironmentID(int(idFloat)), environmentParams
}

func assertEnvironmentArrayInResponse(t *testing.T, response *http.Response) []interface{} {
	t.Helper()
	paramMap := make(map[string]interface{})
	err := json.NewDecoder(response.Body).Decode(&paramMap)
	require.NoError(t, err)
	environments, ok := paramMap["executionEnvironments"]
	assert.True(t, ok)
	environmentsArray, ok := environments.([]interface{})
	assert.True(t, ok)
	return environmentsArray
}

func getEnvironmentFromResponse(t *testing.T, response *http.Response) interface{} {
	t.Helper()
	var environment interface{}
	err := json.NewDecoder(response.Body).Decode(&environment)
	require.NoError(t, err)
	return environment
}

//nolint:unparam // Because its more clear if the environment id is written in the real test
func deleteEnvironment(t *testing.T, id string) {
	t.Helper()
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, id)
	_, err := helpers.HTTPDelete(path, nil)
	require.NoError(t, err)
}

func cleanupJobsForEnvironment(t *testing.T, environmentID string) {
	t.Helper()

	jobListStub, _, err := nomadClient.Jobs().List(&nomadApi.QueryOptions{Prefix: environmentID})
	if err != nil {
		t.Fatalf("Error when listing test jobs: %v", err)
	}

	for _, j := range jobListStub {
		_, _, err := nomadClient.Jobs().DeregisterOpts(j.ID, &nomadApi.DeregisterOptions{Purge: true}, nil)
		if err != nil {
			t.Fatalf("Error when removing test job %v", err)
		}
	}
}

//nolint:unparam // Because its more clear if the environment id is written in the real test
func createEnvironment(t *testing.T, environmentID string) {
	t.Helper()
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, environmentID)
	request := dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 1,
		CPULimit:           100,
		MemoryLimit:        100,
		Image:              *testDockerImage,
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}
	assertPutReturnsStatusAndZeroContent(t, path, request, http.StatusCreated)
}

func assertPutReturnsStatusAndZeroContent(t *testing.T, path string,
	request dto.ExecutionEnvironmentRequest, status int) {
	t.Helper()
	resp, err := helpers.HTTPPutJSON(path, request)
	require.Nil(t, err)
	assert.Equal(t, status, resp.StatusCode)
	assert.Equal(t, int64(0), resp.ContentLength)
	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Empty(t, string(content))
	_ = resp.Body.Close()
}

func validateJob(t *testing.T, expected dto.ExecutionEnvironmentRequest) {
	t.Helper()
	job := findTemplateJob(t, tests.AnotherEnvironmentIDAsInteger)

	assertEqualValueStringPointer(t, nomadNamespace, job.Namespace)
	assertEqualValueStringPointer(t, "batch", job.Type)
	require.Equal(t, 2, len(job.TaskGroups))

	taskGroup := job.TaskGroups[0]
	require.NotNil(t, taskGroup.Count)
	// Poseidon might have already scaled the registered job up once.
	prewarmingPoolSizeInt := int(expected.PrewarmingPoolSize)
	taskGroupCount := *taskGroup.Count
	assert.True(t, prewarmingPoolSizeInt == taskGroupCount || prewarmingPoolSizeInt+1 == taskGroupCount)
	assertEqualValueIntPointer(t, int(expected.PrewarmingPoolSize), taskGroup.Count)
	require.Equal(t, 1, len(taskGroup.Tasks))

	task := taskGroup.Tasks[0]
	assertEqualValueIntPointer(t, int(expected.CPULimit), task.Resources.CPU)
	assertEqualValueIntPointer(t, int(expected.MemoryLimit), task.Resources.MemoryMB)
	assert.Equal(t, expected.Image, task.Config["image"])

	if expected.NetworkAccess {
		assert.Equal(t, "", task.Config["network_mode"])
		require.Equal(t, 1, len(taskGroup.Networks))
		network := taskGroup.Networks[0]
		assert.Equal(t, len(expected.ExposedPorts), len(network.DynamicPorts))
		for _, port := range network.DynamicPorts {
			assert.Contains(t, expected.ExposedPorts, uint16(port.To))
		}
	} else {
		assert.Equal(t, "none", task.Config["network_mode"])
		assert.Equal(t, 0, len(taskGroup.Networks))
	}
}

func findTemplateJob(t *testing.T, id dto.EnvironmentID) *nomadApi.Job {
	t.Helper()
	job, _, err := nomadClient.Jobs().Info(nomad.TemplateJobID(id), nil)
	if err != nil {
		t.Fatalf("Error retrieving Nomad job: %v", err)
	}
	return job
}

func assertEqualValueStringPointer(t *testing.T, value string, valueP *string) {
	t.Helper()
	require.NotNil(t, valueP)
	assert.Equal(t, value, *valueP)
}

func assertEqualValueIntPointer(t *testing.T, value int, valueP *int) {
	t.Helper()
	require.NotNil(t, valueP)
	assert.Equal(t, value, *valueP)
}
