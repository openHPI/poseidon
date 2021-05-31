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
func (nc *nomadApiClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nc.queryOptions)
	return
}

// JobScale returns the scale of the passed job.
func (nc *nomadApiClient) JobScale(jobId string) (jobScale uint, err error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobId, nc.queryOptions)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = uint(status.TaskGroups[fmt.Sprintf(TaskGroupNameFormat, jobId)].Running)
	return
}

// SetJobScale sets the scaling count of the passed job to Nomad.
func (nc *nomadApiClient) SetJobScale(jobId string, count uint, reason string) (err error) {
	intCount := int(count)
	_, _, err = nc.client.Jobs().Scale(jobId, fmt.Sprintf(TaskGroupNameFormat, jobId), &intCount, reason, false, nil, nil)
	return
}
