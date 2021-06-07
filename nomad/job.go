package nomad

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
)

const (
	TaskGroupNameFormat = "%s-group"
	TaskName            = "default-task"
)

// LoadJobList loads the list of jobs from the Nomad api.
func (nc *nomadAPIClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nc.queryOptions)
	return
}

// JobScale returns the scale of the passed job.
func (nc *nomadAPIClient) JobScale(jobID string) (jobScale uint, err error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobID, nc.queryOptions)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = uint(status.TaskGroups[fmt.Sprintf(TaskGroupNameFormat, jobID)].Running)
	return
}

// SetJobScale sets the scaling count of the passed job to Nomad.
func (nc *nomadAPIClient) SetJobScale(jobID string, count uint, reason string) (err error) {
	intCount := int(count)
	_, _, err = nc.client.Jobs().Scale(jobID, fmt.Sprintf(TaskGroupNameFormat, jobID), &intCount, reason, false, nil, nil)
	return
}
