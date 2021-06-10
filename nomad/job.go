package nomad

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"strings"
)

const (
	TaskGroupName            = "default-group"
	TaskName                 = "default-task"
	ConfigTaskGroupName      = "config"
	ConfigTaskName           = "config"
	defaultRunnerJobID       = "default"
	runnerJobIDFormat        = "%s-%s"
	DefaultTaskDriver        = "docker"
	DefaultConfigTaskDriver  = "exec"
	DefaultConfigTaskCommand = "whoami"
	ConfigMetaEnvironmentKey = "environment"
	ConfigMetaUsedKey        = "used"
	ConfigMetaUsedValue      = "true"
	ConfigMetaUnusedValue    = "false"
	ConfigMetaPoolSizeKey    = "prewarmingPoolSize"
)

func DefaultJobID(id string) string {
	return RunnerJobID(id, defaultRunnerJobID)
}

func RunnerJobID(environmentID, runnerID string) string {
	return fmt.Sprintf(runnerJobIDFormat, environmentID, runnerID)
}

func IsDefaultJobID(jobID string) bool {
	parts := strings.Split(jobID, "-")
	return len(parts) == 2 && parts[1] == defaultRunnerJobID
}

func FindConfigTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	for _, tg := range job.TaskGroups {
		if *tg.Name == ConfigTaskGroupName {
			return tg
		}
	}
	return nil
}

func EnvironmentIDFromJobID(jobID string) string {
	parts := strings.Split(jobID, "-")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func (nc *nomadAPIClient) jobInfo(jobID string) (job *nomadApi.Job, err error) {
	job, _, err = nc.client.Jobs().Info(jobID, nil)
	return
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
