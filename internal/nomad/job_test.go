package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests/helpers"
)

func (s *MainTestSuite) TestFindTaskGroup() {
	s.Run("Returns nil if task group not found", func() {
		group := FindTaskGroup(&nomadApi.Job{}, TaskGroupName)
		s.Nil(group)
	})

	s.Run("Finds task group when existent", func() {
		_, job := helpers.CreateTemplateJob()
		group := FindTaskGroup(job, TaskGroupName)
		s.NotNil(group)
	})
}

func (s *MainTestSuite) TestFindOrCreateDefaultTask() {
	s.Run("Adds default task group when not set", func() {
		job := &nomadApi.Job{}
		group := FindAndValidateDefaultTaskGroup(job)
		s.NotNil(group)
		s.Equal(TaskGroupName, *group.Name)
		s.Equal(1, len(job.TaskGroups))
		s.Equal(group, job.TaskGroups[0])
		s.Equal(TaskCount, *group.Count)
	})

	s.Run("Does not modify task group when already set", func() {
		job := &nomadApi.Job{}
		groupName := TaskGroupName
		expectedGroup := &nomadApi.TaskGroup{Name: &groupName}
		job.TaskGroups = []*nomadApi.TaskGroup{expectedGroup}

		group := FindAndValidateDefaultTaskGroup(job)
		s.NotNil(group)
		s.Equal(1, len(job.TaskGroups))
		s.Equal(expectedGroup, group)
	})
}

func (s *MainTestSuite) TestFindOrCreateConfigTaskGroup() {
	s.Run("Adds config task group when not set", func() {
		job := &nomadApi.Job{}
		group := FindAndValidateConfigTaskGroup(job)
		s.NotNil(group)
		s.Equal(group, job.TaskGroups[0])
		s.Equal(1, len(job.TaskGroups))

		s.Equal(ConfigTaskGroupName, *group.Name)
		s.Equal(0, *group.Count)
	})

	s.Run("Does not modify task group when already set", func() {
		job := &nomadApi.Job{}
		groupName := ConfigTaskGroupName
		expectedGroup := &nomadApi.TaskGroup{Name: &groupName}
		job.TaskGroups = []*nomadApi.TaskGroup{expectedGroup}

		group := FindAndValidateConfigTaskGroup(job)
		s.NotNil(group)
		s.Equal(1, len(job.TaskGroups))
		s.Equal(expectedGroup, group)
	})
}

func (s *MainTestSuite) TestFindOrCreateTask() {
	s.Run("Does not modify default task when already set", func() {
		groupName := TaskGroupName
		group := &nomadApi.TaskGroup{Name: &groupName}
		expectedTask := &nomadApi.Task{Name: TaskName}
		group.Tasks = []*nomadApi.Task{expectedTask}

		task := FindAndValidateDefaultTask(group)
		s.NotNil(task)
		s.Equal(1, len(group.Tasks))
		s.Equal(expectedTask, task)
	})

	s.Run("Does not modify config task when already set", func() {
		groupName := ConfigTaskGroupName
		group := &nomadApi.TaskGroup{Name: &groupName}
		expectedTask := &nomadApi.Task{Name: ConfigTaskName}
		group.Tasks = []*nomadApi.Task{expectedTask}

		task := FindAndValidateConfigTask(group)
		s.NotNil(task)
		s.Equal(1, len(group.Tasks))
		s.Equal(expectedTask, task)
	})
}

func (s *MainTestSuite) TestSetForcePullFlag() {
	_, job := helpers.CreateTemplateJob()
	taskGroup := FindAndValidateDefaultTaskGroup(job)
	task := FindAndValidateDefaultTask(taskGroup)

	s.Run("Ignoring passed value if DisableForcePull", func() {
		config.Config.Nomad.DisableForcePull = true
		SetForcePullFlag(job, true)
		s.Equal(false, task.Config["force_pull"])
	})

	s.Run("Using passed value if not DisableForcePull", func() {
		config.Config.Nomad.DisableForcePull = false

		SetForcePullFlag(job, true)
		s.Equal(true, task.Config["force_pull"])

		SetForcePullFlag(job, false)
		s.Equal(false, task.Config["force_pull"])
	})
}

func (s *MainTestSuite) TestIsEnvironmentTemplateID() {
	s.True(IsEnvironmentTemplateID("template-42"))
	s.False(IsEnvironmentTemplateID("template-42-100"))
	s.False(IsEnvironmentTemplateID("job-42"))
	s.False(IsEnvironmentTemplateID("template-top"))
}

func (s *MainTestSuite) TestRunnerJobID() {
	s.Equal("0-RANDOM-UUID", RunnerJobID(0, "RANDOM-UUID"))
}

func (s *MainTestSuite) TestTemplateJobID() {
	s.Equal("template-42", TemplateJobID(42))
}

func (s *MainTestSuite) TestEnvironmentIDFromRunnerID() {
	id, err := EnvironmentIDFromRunnerID("42-RANDOM-UUID")
	s.Require().NoError(err)
	s.Equal(dto.EnvironmentID(42), id)

	_, err = EnvironmentIDFromRunnerID("")
	s.Error(err)
}

func (s *MainTestSuite) TestOOMKilledAllocation() {
	event := nomadApi.TaskEvent{Details: map[string]string{"oom_killed": "true"}}
	state := nomadApi.TaskState{Restarts: 2, Events: []*nomadApi.TaskEvent{&event}}
	alloc := nomadApi.Allocation{TaskStates: map[string]*nomadApi.TaskState{TaskName: &state}}
	s.False(isOOMKilled(&alloc))

	event2 := nomadApi.TaskEvent{Details: map[string]string{"oom_killed": "false"}}
	alloc.TaskStates[TaskName].Events = []*nomadApi.TaskEvent{&event, &event2}
	s.False(isOOMKilled(&alloc))

	event3 := nomadApi.TaskEvent{Details: map[string]string{"oom_killed": "true"}}
	alloc.TaskStates[TaskName].Events = []*nomadApi.TaskEvent{&event, &event2, &event3}
	s.True(isOOMKilled(&alloc))
}
