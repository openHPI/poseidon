package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"strconv"
	"strings"
	"time"
)

const (
	TemplateJobPrefix     = "template"
	TaskGroupName         = "default-group"
	TaskName              = "default-task"
	TaskCount             = 1
	TaskDriver            = "docker"
	TaskCommand           = "sleep"
	ConfigTaskGroupName   = "config"
	ConfigTaskName        = "config"
	ConfigTaskDriver      = "exec"
	ConfigTaskCommand     = "true"
	ConfigMetaUsedKey     = "used"
	ConfigMetaUsedValue   = "true"
	ConfigMetaUnusedValue = "false"
	ConfigMetaTimeoutKey  = "timeout"
	ConfigMetaPoolSizeKey = "prewarmingPoolSize"
	TemplateJobNameParts  = 2
	RegisterTimeout       = 10 * time.Second
)

var (
	ErrorInvalidJobID = errors.New("invalid job id")
	TaskArgs          = []string{"infinity"}
)

func (a *APIClient) RegisterRunnerJob(template *nomadApi.Job) error {
	taskGroup := FindOrCreateConfigTaskGroup(template)

	taskGroup.Meta = make(map[string]string)
	taskGroup.Meta[ConfigMetaUsedKey] = ConfigMetaUnusedValue

	evalID, err := a.apiQuerier.RegisterNomadJob(template)
	if err != nil {
		return fmt.Errorf("couldn't register runner job: %w", err)
	}

	registerTimeout, cancel := context.WithTimeout(context.Background(), RegisterTimeout)
	defer cancel()
	return a.MonitorEvaluation(evalID, registerTimeout)
}

func FindTaskGroup(job *nomadApi.Job, name string) *nomadApi.TaskGroup {
	for _, tg := range job.TaskGroups {
		if *tg.Name == name {
			return tg
		}
	}
	return nil
}

func FindOrCreateDefaultTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	taskGroup := FindTaskGroup(job, TaskGroupName)
	if taskGroup == nil {
		taskGroup = nomadApi.NewTaskGroup(TaskGroupName, TaskCount)
		job.AddTaskGroup(taskGroup)
	}
	FindOrCreateDefaultTask(taskGroup)
	return taskGroup
}

func FindOrCreateConfigTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	taskGroup := FindTaskGroup(job, ConfigTaskGroupName)
	if taskGroup == nil {
		taskGroup = nomadApi.NewTaskGroup(ConfigTaskGroupName, 0)
		job.AddTaskGroup(taskGroup)
	}
	FindOrCreateConfigTask(taskGroup)
	return taskGroup
}

// FindOrCreateConfigTask ensures that a dummy task is in the task group so that the group is accepted by Nomad.
func FindOrCreateConfigTask(taskGroup *nomadApi.TaskGroup) *nomadApi.Task {
	var task *nomadApi.Task
	for _, t := range taskGroup.Tasks {
		if t.Name == ConfigTaskName {
			task = t
			break
		}
	}

	if task == nil {
		task = nomadApi.NewTask(ConfigTaskName, ConfigTaskDriver)
		taskGroup.Tasks = append(taskGroup.Tasks, task)
	}

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["command"] = ConfigTaskCommand
	return task
}

// FindOrCreateDefaultTask ensures that a default task is in the task group in that the executions are made.
func FindOrCreateDefaultTask(taskGroup *nomadApi.TaskGroup) *nomadApi.Task {
	var task *nomadApi.Task
	for _, t := range taskGroup.Tasks {
		if t.Name == TaskName {
			task = t
			break
		}
	}

	if task == nil {
		task = nomadApi.NewTask(TaskName, TaskDriver)
		taskGroup.Tasks = append(taskGroup.Tasks, task)
	}

	if task.Resources == nil {
		task.Resources = nomadApi.DefaultResources()
	}

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["command"] = TaskCommand
	task.Config["args"] = TaskArgs
	return task
}

// IsEnvironmentTemplateID checks if the passed job id belongs to a template job.
func IsEnvironmentTemplateID(jobID string) bool {
	parts := strings.Split(jobID, "-")
	if len(parts) != TemplateJobNameParts || parts[0] != TemplateJobPrefix {
		return false
	}

	_, err := EnvironmentIDFromTemplateJobID(jobID)
	return err == nil
}

// RunnerJobID returns the nomad job id of the runner with the given environmentID and id.
func RunnerJobID(environmentID dto.EnvironmentID, id string) string {
	return fmt.Sprintf("%d-%s", environmentID, id)
}

// TemplateJobID returns the id of the nomad job for the environment with the given id.
func TemplateJobID(id dto.EnvironmentID) string {
	return fmt.Sprintf("%s-%d", TemplateJobPrefix, id)
}

// EnvironmentIDFromRunnerID returns the environment id that is part of the passed runner job id.
func EnvironmentIDFromRunnerID(jobID string) (dto.EnvironmentID, error) {
	return partOfJobID(jobID, 0)
}

// EnvironmentIDFromTemplateJobID returns the environment id that is part of the passed environment job id.
func EnvironmentIDFromTemplateJobID(id string) (dto.EnvironmentID, error) {
	return partOfJobID(id, 1)
}

func partOfJobID(id string, part uint) (dto.EnvironmentID, error) {
	parts := strings.Split(id, "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty job id: %w", ErrorInvalidJobID)
	}
	environmentID, err := strconv.Atoi(parts[part])
	if err != nil {
		return 0, fmt.Errorf("invalid environment id par %v: %w", err, ErrorInvalidJobID)
	}
	return dto.EnvironmentID(environmentID), nil
}
