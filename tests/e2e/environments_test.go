package e2e

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"net/http"
	"strings"
	"testing"
)

const (
	dockerImage = "python:latest"
)

func TestCreateOrUpdateEnvironment(t *testing.T) {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.AnotherEnvironmentIdAsString)

	t.Run("returns bad request with empty body", func(t *testing.T) {
		resp, err := helpers.HttpPut(path, strings.NewReader(""))
		require.Nil(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		_ = resp.Body.Close()
	})

	request := dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 1,
		CPULimit:           100,
		MemoryLimit:        100,
		Image:              dockerImage,
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	t.Run("creates correct environment in Nomad", func(t *testing.T) {
		assertPutReturnsStatusAndZeroContent(t, path, request, http.StatusCreated)
		validateJob(t, request)
	})

	t.Run("updates limits in Nomad correctly", func(t *testing.T) {
		updateRequest := request
		updateRequest.CPULimit = 1337
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

	_, _, err := nomadClient.Jobs().DeregisterOpts(
		tests.AnotherEnvironmentIdAsString, &nomadApi.DeregisterOptions{Purge: true}, nil)
	if err != nil {
		t.Fatalf("Error when removing test job %v", err)
	}
}

func assertPutReturnsStatusAndZeroContent(t *testing.T, path string,
	request dto.ExecutionEnvironmentRequest, status int) {
	t.Helper()
	resp, err := helpers.HttpPutJSON(path, request)
	require.Nil(t, err)
	assert.Equal(t, status, resp.StatusCode)
	assert.Equal(t, int64(0), resp.ContentLength)

	_ = resp.Body.Close()
}

func validateJob(t *testing.T, expected dto.ExecutionEnvironmentRequest) {
	t.Helper()
	job := findNomadJob(t, tests.AnotherEnvironmentIdAsString)

	assertEqualValueStringPointer(t, nomadNamespace, job.Namespace)
	assertEqualValueStringPointer(t, "batch", job.Type)
	require.Equal(t, 1, len(job.TaskGroups))

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

func findNomadJob(t *testing.T, jobID string) *nomadApi.Job {
	t.Helper()
	job, _, err := nomadClient.Jobs().Info(jobID, nil)
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
