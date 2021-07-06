package nomad

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"strconv"
	"testing"
)

func createTestTaskGroup() *nomadApi.TaskGroup {
	return nomadApi.NewTaskGroup("taskGroup", 42)
}

func createTestTask() *nomadApi.Task {
	return nomadApi.NewTask("task", "docker")
}

func createTestResources() *nomadApi.Resources {
	expectedCPULimit := 1337
	expectedMemoryLimit := 42
	return &nomadApi.Resources{CPU: &expectedCPULimit, MemoryMB: &expectedMemoryLimit}
}

func TestCreateTaskGroupCreatesNewTaskGroupWhenJobHasNoTaskGroup(t *testing.T) {
	job := nomadApi.NewBatchJob("test", "test", "test", 1)

	if assert.Equal(t, 0, len(job.TaskGroups)) {
		expectedTaskGroup := createTestTaskGroup()
		taskGroup := createTaskGroup(job, *expectedTaskGroup.Name, uint(*expectedTaskGroup.Count))

		assert.Equal(t, *expectedTaskGroup, *taskGroup)
		assert.Equal(t, []*nomadApi.TaskGroup{taskGroup}, job.TaskGroups, "it should add the task group to the job")
	}
}

func TestCreateTaskGroupOverwritesOptionsWhenJobHasTaskGroup(t *testing.T) {
	job := nomadApi.NewBatchJob("test", "test", "test", 1)
	existingTaskGroup := createTestTaskGroup()
	existingTaskGroup.Meta = map[string]string{"field": "should still exist"}
	newTaskGroupList := []*nomadApi.TaskGroup{existingTaskGroup}
	job.TaskGroups = newTaskGroupList

	newName := *existingTaskGroup.Name + "longerName"
	newCount := *existingTaskGroup.Count + 42

	taskGroup := createTaskGroup(job, newName, uint(newCount))

	// create a new copy to avoid changing the original one as it is a pointer
	expectedTaskGroup := *existingTaskGroup
	expectedTaskGroup.Name = &newName

	assert.Equal(t, expectedTaskGroup, *taskGroup)
	assert.Equal(t, newTaskGroupList, job.TaskGroups, "it should not modify the jobs task group list")
}

func TestConfigureNetworkFatalsWhenNoTaskExists(t *testing.T) {
	logger, hook := test.NewNullLogger()
	logger.ExitFunc = func(i int) {
		panic(i)
	}
	log = logger.WithField("pkg", "job_test")
	taskGroup := createTestTaskGroup()
	if assert.Equal(t, 0, len(taskGroup.Tasks)) {
		assert.Panics(t, func() {
			configureNetwork(taskGroup, false, nil)
		})
		assert.Equal(t, logrus.FatalLevel, hook.LastEntry().Level)
	}
}

func TestConfigureNetworkCreatesNewNetworkWhenNoNetworkExists(t *testing.T) {
	taskGroup := createTestTaskGroup()
	task := createTestTask()
	taskGroup.Tasks = []*nomadApi.Task{task}

	if assert.Equal(t, 0, len(taskGroup.Networks)) {
		configureNetwork(taskGroup, true, []uint16{})

		assert.Equal(t, 1, len(taskGroup.Networks))
	}
}

func TestConfigureNetworkDoesNotCreateNewNetworkWhenNetworkExists(t *testing.T) {
	taskGroup := createTestTaskGroup()
	task := createTestTask()
	taskGroup.Tasks = []*nomadApi.Task{task}
	networkResource := &nomadApi.NetworkResource{Mode: "bridge"}
	taskGroup.Networks = []*nomadApi.NetworkResource{networkResource}

	if assert.Equal(t, 1, len(taskGroup.Networks)) {
		configureNetwork(taskGroup, true, []uint16{})

		assert.Equal(t, 1, len(taskGroup.Networks))
		assert.Equal(t, networkResource, taskGroup.Networks[0])
	}
}

func TestConfigureNetworkSetsCorrectValues(t *testing.T) {
	taskGroup := createTestTaskGroup()
	task := createTestTask()
	_, ok := task.Config["network_mode"]

	require.False(t, ok, "Test tasks network_mode should not be set")

	taskGroup.Tasks = []*nomadApi.Task{task}
	exposedPortsTests := [][]uint16{{}, {1337}, {42, 1337}}

	t.Run("with no network access", func(t *testing.T) {
		for _, ports := range exposedPortsTests {
			testTaskGroup := *taskGroup
			testTask := *task
			testTaskGroup.Tasks = []*nomadApi.Task{&testTask}

			configureNetwork(&testTaskGroup, false, ports)
			mode, ok := testTask.Config["network_mode"]
			assert.True(t, ok)
			assert.Equal(t, "none", mode)
			assert.Equal(t, 0, len(testTaskGroup.Networks))
		}
	})

	t.Run("with network access", func(t *testing.T) {
		for _, ports := range exposedPortsTests {
			testTaskGroup := *taskGroup
			testTask := *task
			testTaskGroup.Tasks = []*nomadApi.Task{&testTask}

			configureNetwork(&testTaskGroup, true, ports)
			require.Equal(t, 1, len(testTaskGroup.Networks))

			networkResource := testTaskGroup.Networks[0]
			assert.Equal(t, "bridge", networkResource.Mode)
			require.Equal(t, len(ports), len(networkResource.DynamicPorts))

			for _, expectedPort := range ports {
				found := false
				for _, actualPort := range networkResource.DynamicPorts {
					if actualPort.To == int(expectedPort) {
						found = true
						break
					}
				}
				assert.True(t, found, fmt.Sprintf("port list should contain %v", expectedPort))
			}

			mode, ok := testTask.Config["network_mode"]
			assert.True(t, ok)
			assert.Equal(t, mode, "")
		}
	})
}

func TestConfigureTaskWhenNoTaskExists(t *testing.T) {
	taskGroup := createTestTaskGroup()
	require.Equal(t, 0, len(taskGroup.Tasks))

	expectedResources := createTestResources()
	expectedTaskGroup := *taskGroup
	expectedTask := nomadApi.NewTask("task", TaskDriver)
	expectedTask.Resources = expectedResources
	expectedImage := "python:latest"
	expectedTask.Config = map[string]interface{}{"image": expectedImage, "network_mode": "none"}
	expectedTaskGroup.Tasks = []*nomadApi.Task{expectedTask}
	expectedTaskGroup.Networks = []*nomadApi.NetworkResource{}

	configureTask(taskGroup, expectedTask.Name,
		uint(*expectedResources.CPU), uint(*expectedResources.MemoryMB),
		expectedImage, false, []uint16{})

	assert.Equal(t, expectedTaskGroup, *taskGroup)
}

func TestConfigureTaskWhenTaskExists(t *testing.T) {
	taskGroup := createTestTaskGroup()
	task := createTestTask()
	task.Config = map[string]interface{}{"my_custom_config": "should not be overwritten"}
	taskGroup.Tasks = []*nomadApi.Task{task}
	require.Equal(t, 1, len(taskGroup.Tasks))

	expectedResources := createTestResources()
	expectedTaskGroup := *taskGroup
	expectedTask := *task
	expectedTask.Resources = expectedResources
	expectedImage := "python:latest"
	expectedTask.Config["image"] = expectedImage
	expectedTask.Config["network_mode"] = "none"
	expectedTaskGroup.Tasks = []*nomadApi.Task{&expectedTask}
	expectedTaskGroup.Networks = []*nomadApi.NetworkResource{}

	configureTask(taskGroup, expectedTask.Name,
		uint(*expectedResources.CPU), uint(*expectedResources.MemoryMB),
		expectedImage, false, []uint16{})

	assert.Equal(t, expectedTaskGroup, *taskGroup)
	assert.Equal(t, task, taskGroup.Tasks[0], "it should not create a new task")
}

func TestCreateTemplateJobSetsAllGivenArguments(t *testing.T) {
	testJob := helpers.CreateTemplateJob()
	prewarmingPoolSize, err := strconv.Atoi(testJob.TaskGroups[1].Meta[ConfigMetaPoolSizeKey])
	require.NoError(t, err)
	job := CreateTemplateJob(
		helpers.CreateTemplateJob(),
		tests.DefaultJobID,
		uint(prewarmingPoolSize),
		uint(*testJob.TaskGroups[0].Tasks[0].Resources.CPU),
		uint(*testJob.TaskGroups[0].Tasks[0].Resources.MemoryMB),
		testJob.TaskGroups[0].Tasks[0].Config["image"].(string),
		false,
		nil,
	)
	assert.Equal(t, *testJob, *job)
}

func TestRegisterTemplateJobFailsWhenNomadJobRegistrationFails(t *testing.T) {
	apiMock := apiQuerierMock{}
	expectedErr := tests.ErrDefault

	apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return("", expectedErr)

	apiClient := &APIClient{&apiMock}

	_, err := apiClient.RegisterTemplateJob(&nomadApi.Job{}, tests.DefaultJobID,
		1, 2, 3, "image", false, []uint16{})
	assert.ErrorIs(t, err, expectedErr)
	apiMock.AssertNotCalled(t, "EvaluationStream")
}

func TestRegisterTemplateJobSucceedsWhenMonitoringEvaluationSucceeds(t *testing.T) {
	apiMock := apiQuerierMock{}
	evaluationID := "id"

	stream := make(chan *nomadApi.Events)
	readonlyStream := func() <-chan *nomadApi.Events {
		return stream
	}()
	// Immediately close stream to avoid any reading from it resulting in endless wait
	close(stream)

	apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiMock.On("EvaluationStream", evaluationID, mock.AnythingOfType("*context.emptyCtx")).
		Return(readonlyStream, nil)

	apiClient := &APIClient{&apiMock}

	_, err := apiClient.RegisterTemplateJob(&nomadApi.Job{}, tests.DefaultJobID,
		1, 2, 3, "image", false, []uint16{})
	assert.NoError(t, err)
}

func TestRegisterTemplateJobReturnsErrorWhenMonitoringEvaluationFails(t *testing.T) {
	apiMock := apiQuerierMock{}
	evaluationID := "id"

	apiMock.On("RegisterNomadJob", mock.AnythingOfType("*api.Job")).Return(evaluationID, nil)
	apiMock.On("EvaluationStream", evaluationID, mock.AnythingOfType("*context.emptyCtx")).Return(nil, tests.ErrDefault)

	apiClient := &APIClient{&apiMock}

	_, err := apiClient.RegisterTemplateJob(&nomadApi.Job{}, tests.DefaultJobID,
		1, 2, 3, "image", false, []uint16{})
	assert.ErrorIs(t, err, tests.ErrDefault)
}
