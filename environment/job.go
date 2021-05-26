package environment

import (
	"context"
	_ "embed"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"strconv"
)

const (
	DefaultTaskDriver = "docker"
	TaskNameFormat    = "%s-task"
)

// defaultJobHCL holds our default job in HCL format.
// The default job is used when creating new job and provides
// common settings that all the jobs share.
//go:embed default-job.hcl
var defaultJobHCL string

// registerJob creates a Nomad job based on the default job configuration and the given parameters.
// It registers the job with Nomad and waits until the registration completes.
func (m *NomadEnvironmentManager) registerJob(
	id string,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) error {
	job := createJob(m.defaultJob, id, prewarmingPoolSize, cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
	evalID, err := m.api.RegisterNomadJob(job)
	if err != nil {
		return err
	}
	return m.api.MonitorEvaluation(evalID, context.Background())
}

func createJob(
	defaultJob nomadApi.Job,
	id string,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) *nomadApi.Job {

	job := defaultJob
	job.ID = &id
	job.Name = &id

	var taskGroup = createTaskGroup(&job, fmt.Sprintf(nomad.TaskGroupNameFormat, id), prewarmingPoolSize)
	configureTask(taskGroup, fmt.Sprintf(TaskNameFormat, id), cpuLimit, memoryLimit, image, networkAccess, exposedPorts)

	return &job
}

func parseJob(jobHCL string) *nomadApi.Job {
	config := jobspec2.ParseConfig{
		Body:    []byte(jobHCL),
		AllowFS: false,
		Strict:  true,
	}
	job, err := jobspec2.ParseWithConfig(&config)
	if err != nil {
		log.WithError(err).Fatal("Error parsing Nomad job")
		return nil
	}

	return job
}

func createTaskGroup(job *nomadApi.Job, name string, prewarmingPoolSize uint) *nomadApi.TaskGroup {
	var taskGroup *nomadApi.TaskGroup
	if len(job.TaskGroups) == 0 {
		taskGroup = nomadApi.NewTaskGroup(name, int(prewarmingPoolSize))
		job.TaskGroups = []*nomadApi.TaskGroup{taskGroup}
	} else {
		taskGroup = job.TaskGroups[0]
		taskGroup.Name = &name
		count := int(prewarmingPoolSize)
		taskGroup.Count = &count
	}
	return taskGroup
}

func configureNetwork(taskGroup *nomadApi.TaskGroup, networkAccess bool, exposedPorts []uint16) {
	if len(taskGroup.Tasks) == 0 {
		// This function is only used internally and must be called as last step when configuring the task.
		// This error is not recoverable.
		log.Fatal("Can't configure network before task has been configured!")
	}
	task := taskGroup.Tasks[0]

	if networkAccess {
		var networkResource *nomadApi.NetworkResource
		if len(taskGroup.Networks) == 0 {
			networkResource = &nomadApi.NetworkResource{}
			taskGroup.Networks = []*nomadApi.NetworkResource{networkResource}
		} else {
			networkResource = taskGroup.Networks[0]
		}
		// Prefer "bridge" network over "host" to have an isolated network namespace with bridged interface
		// instead of joining the host network namespace.
		networkResource.Mode = "bridge"
		for _, portNumber := range exposedPorts {
			port := nomadApi.Port{
				Label: strconv.FormatUint(uint64(portNumber), 10),
				To:    int(portNumber),
			}
			networkResource.DynamicPorts = append(networkResource.DynamicPorts, port)
		}
	} else {
		// Somehow, we can't set the network mode to none in the NetworkResource on task group level.
		// See https://github.com/hashicorp/nomad/issues/10540
		if task.Config == nil {
			task.Config = make(map[string]interface{})
		}
		task.Config["network_mode"] = "none"
	}
}

func configureTask(
	taskGroup *nomadApi.TaskGroup,
	name string,
	cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) {
	var task *nomadApi.Task
	if len(taskGroup.Tasks) == 0 {
		task = nomadApi.NewTask(name, DefaultTaskDriver)
		taskGroup.Tasks = []*nomadApi.Task{task}
	} else {
		task = taskGroup.Tasks[0]
		task.Name = name
	}
	integerCpuLimit := int(cpuLimit)
	integerMemoryLimit := int(memoryLimit)
	task.Resources = &nomadApi.Resources{
		CPU:      &integerCpuLimit,
		MemoryMB: &integerMemoryLimit,
	}

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["image"] = image

	configureNetwork(taskGroup, networkAccess, exposedPorts)
}