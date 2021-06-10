package nomad

import (
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"strconv"
	"strings"
)

const (
	TaskGroupName            = "default-group"
	TaskName                 = "default-task"
	ConfigTaskGroupName      = "config"
	DummyTaskName            = "dummy"
	defaultRunnerJobID       = "default"
	DefaultTaskDriver        = "docker"
	DefaultDummyTaskDriver   = "exec"
	DefaultTaskCommand       = "true"
	ConfigMetaEnvironmentKey = "environment"
	ConfigMetaUsedKey        = "used"
	ConfigMetaUsedValue      = "true"
	ConfigMetaUnusedValue    = "false"
	ConfigMetaPoolSizeKey    = "prewarmingPoolSize"
)

var (
	ErrorInvalidJobID            = errors.New("invalid job id")
	ErrorConfigTaskGroupNotFound = errors.New("config task group not found in job")
)

// RunnerJobID creates the job id. This requires an environment id and a runner id.
func RunnerJobID(environmentID, runnerID string) string {
	return fmt.Sprintf("%s-%s", environmentID, runnerID)
}

// EnvironmentIDFromJobID returns the environment id that is part of the passed job id.
func EnvironmentIDFromJobID(jobID string) (int, error) {
	parts := strings.Split(jobID, "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty job id: %w", ErrorInvalidJobID)
	}
	environmentID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid environment id par %v: %w", err, ErrorInvalidJobID)
	}
	return environmentID, nil
}

// TemplateJobID creates the environment specific id of the template job.
func TemplateJobID(id string) string {
	return RunnerJobID(id, defaultRunnerJobID)
}

// IsEnvironmentTemplateID checks if the passed job id belongs to a template job.
func IsEnvironmentTemplateID(jobID string) bool {
	parts := strings.Split(jobID, "-")
	return len(parts) == 2 && parts[1] == defaultRunnerJobID
}

// FindConfigTaskGroup returns the config task group of a job.
// The config task group should be included in all jobs.
func FindConfigTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	for _, tg := range job.TaskGroups {
		if *tg.Name == ConfigTaskGroupName {
			return tg
		}
	}
	return nil
}

func SetMetaConfigValue(job *nomadApi.Job, key, value string) error {
	configTaskGroup := FindConfigTaskGroup(job)
	if configTaskGroup == nil {
		return ErrorConfigTaskGroupNotFound
	}
	configTaskGroup.Meta[key] = value
	return nil
}

// LoadJobList loads the list of jobs from the Nomad api.
func (nc *nomadAPIClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nc.queryOptions())
	return
}

// JobScale returns the scale of the passed job.
func (nc *nomadAPIClient) JobScale(jobID string) (jobScale uint, err error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobID, nc.queryOptions())
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = uint(status.TaskGroups[TaskGroupName].Running)
	return
}

// SetJobScale sets the scaling count of the passed job to Nomad.
func (nc *nomadAPIClient) SetJobScale(jobID string, count uint, reason string) (err error) {
	intCount := int(count)
	_, _, err = nc.client.Jobs().Scale(jobID, TaskGroupName, &intCount, reason, false, nil, nil)
	return
}

func (nc *nomadAPIClient) job(jobID string) (job *nomadApi.Job, err error) {
	job, _, err = nc.client.Jobs().Info(jobID, nil)
	return
}
