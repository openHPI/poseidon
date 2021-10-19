package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFindTaskGroup(t *testing.T) {
	t.Run("Returns nil if task group not found", func(t *testing.T) {
		group := FindTaskGroup(&nomadApi.Job{}, TaskGroupName)
		assert.Nil(t, group)
	})

	t.Run("Finds task group when existent", func(t *testing.T) {
		_, job := helpers.CreateTemplateJob()
		group := FindTaskGroup(job, TaskGroupName)
		assert.NotNil(t, group)
	})
}

func TestFindOrCreateDefaultTask(t *testing.T) {
	t.Run("Adds default task group when not set", func(t *testing.T) {
		job := &nomadApi.Job{}
		group := FindOrCreateDefaultTaskGroup(job)
		assert.NotNil(t, group)
		assert.Equal(t, TaskGroupName, *group.Name)
		assert.Equal(t, 1, len(job.TaskGroups))
		assert.Equal(t, group, job.TaskGroups[0])
		assert.Equal(t, TaskCount, *group.Count)
	})

	t.Run("Does not modify task group when already set", func(t *testing.T) {
		job := &nomadApi.Job{}
		groupName := TaskGroupName
		expectedGroup := &nomadApi.TaskGroup{Name: &groupName}
		job.TaskGroups = []*nomadApi.TaskGroup{expectedGroup}

		group := FindOrCreateDefaultTaskGroup(job)
		assert.NotNil(t, group)
		assert.Equal(t, 1, len(job.TaskGroups))
		assert.Equal(t, expectedGroup, group)
	})
}

func TestFindOrCreateConfigTaskGroup(t *testing.T) {
	t.Run("Adds config task group when not set", func(t *testing.T) {
		job := &nomadApi.Job{}
		group := FindOrCreateConfigTaskGroup(job)
		assert.NotNil(t, group)
		assert.Equal(t, group, job.TaskGroups[0])
		assert.Equal(t, 1, len(job.TaskGroups))

		assert.Equal(t, ConfigTaskGroupName, *group.Name)
		assert.Equal(t, 0, *group.Count)
	})

	t.Run("Does not modify task group when already set", func(t *testing.T) {
		job := &nomadApi.Job{}
		groupName := ConfigTaskGroupName
		expectedGroup := &nomadApi.TaskGroup{Name: &groupName}
		job.TaskGroups = []*nomadApi.TaskGroup{expectedGroup}

		group := FindOrCreateConfigTaskGroup(job)
		assert.NotNil(t, group)
		assert.Equal(t, 1, len(job.TaskGroups))
		assert.Equal(t, expectedGroup, group)
	})
}

func TestFindOrCreateTask(t *testing.T) {
	t.Run("Does not modify default task when already set", func(t *testing.T) {
		groupName := TaskGroupName
		group := &nomadApi.TaskGroup{Name: &groupName}
		expectedTask := &nomadApi.Task{Name: TaskName}
		group.Tasks = []*nomadApi.Task{expectedTask}

		task := FindOrCreateDefaultTask(group)
		assert.NotNil(t, task)
		assert.Equal(t, 1, len(group.Tasks))
		assert.Equal(t, expectedTask, task)
	})

	t.Run("Does not modify config task when already set", func(t *testing.T) {
		groupName := ConfigTaskGroupName
		group := &nomadApi.TaskGroup{Name: &groupName}
		expectedTask := &nomadApi.Task{Name: ConfigTaskName}
		group.Tasks = []*nomadApi.Task{expectedTask}

		task := FindOrCreateConfigTask(group)
		assert.NotNil(t, task)
		assert.Equal(t, 1, len(group.Tasks))
		assert.Equal(t, expectedTask, task)
	})
}

func TestIsEnvironmentTemplateID(t *testing.T) {
	assert.True(t, IsEnvironmentTemplateID("template-42"))
	assert.False(t, IsEnvironmentTemplateID("template-42-100"))
	assert.False(t, IsEnvironmentTemplateID("job-42"))
	assert.False(t, IsEnvironmentTemplateID("template-top"))
}

func TestRunnerJobID(t *testing.T) {
	assert.Equal(t, "0-RANDOM-UUID", RunnerJobID(0, "RANDOM-UUID"))
}

func TestTemplateJobID(t *testing.T) {
	assert.Equal(t, "template-42", TemplateJobID(42))
}

func TestEnvironmentIDFromRunnerID(t *testing.T) {
	id, err := EnvironmentIDFromRunnerID("42-RANDOM-UUID")
	assert.NoError(t, err)
	assert.Equal(t, dto.EnvironmentID(42), id)

	_, err = EnvironmentIDFromRunnerID("")
	assert.Error(t, err)
}
