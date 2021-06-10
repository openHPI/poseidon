package environment

import (
	"context"
	_ "embed"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"strconv"
)

// defaultJobHCL holds our default job in HCL format.
// The default job is used when creating new job and provides
// common settings that all the jobs share.
//go:embed default-job.hcl
var defaultJobHCL string

// registerTemplateJob creates a Nomad job based on the default job configuration and the given parameters.
// It registers the job with Nomad and waits until the registration completes.
func (m *NomadEnvironmentManager) registerTemplateJob(
	environmentID runner.EnvironmentID,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) (*nomadApi.Job, error) {
	job := createTemplateJob(m.defaultJob, environmentID, prewarmingPoolSize,
		cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
	evalID, err := m.api.RegisterNomadJob(job)
	if err != nil {
		return nil, err
	}
	return job, m.api.MonitorEvaluation(evalID, context.Background())
}

func createTemplateJob(
	defaultJob nomadApi.Job,
	environmentID runner.EnvironmentID,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) *nomadApi.Job {
	job := defaultJob
	templateJobID := nomad.TemplateJobID(strconv.Itoa(int(environmentID)))
	job.ID = &templateJobID
	job.Name = &templateJobID

	var taskGroup = createTaskGroup(&job, nomad.TaskGroupName, prewarmingPoolSize)
	configureTask(taskGroup, nomad.TaskName, cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
	storeConfiguration(&job, environmentID, prewarmingPoolSize)

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
		count := 1
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

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}

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

		// Explicitly set mode to override existing settings when updating job from without to with network.
		// Don't use bridge as it collides with the bridge mode above. This results in Docker using 'bridge'
		// mode, meaning all allocations will be attached to the `docker0` adapter and could reach other
		// non-Nomad containers attached to it. This is avoided when using Nomads bridge network mode.
		task.Config["network_mode"] = ""
	} else {
		// Somehow, we can't set the network mode to none in the NetworkResource on task group level.
		// See https://github.com/hashicorp/nomad/issues/10540
		task.Config["network_mode"] = "none"
		// Explicitly set Networks to signal Nomad to remove the possibly existing networkResource
		taskGroup.Networks = []*nomadApi.NetworkResource{}
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
		task = nomadApi.NewTask(name, nomad.DefaultTaskDriver)
		taskGroup.Tasks = []*nomadApi.Task{task}
	} else {
		task = taskGroup.Tasks[0]
		task.Name = name
	}
	integerCPULimit := int(cpuLimit)
	integerMemoryLimit := int(memoryLimit)
	task.Resources = &nomadApi.Resources{
		CPU:      &integerCPULimit,
		MemoryMB: &integerMemoryLimit,
	}

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["image"] = image

	configureNetwork(taskGroup, networkAccess, exposedPorts)
}

func storeConfiguration(job *nomadApi.Job, id runner.EnvironmentID, prewarmingPoolSize uint) {
	taskGroup := findOrCreateConfigTaskGroup(job)

	if taskGroup.Meta == nil {
		taskGroup.Meta = make(map[string]string)
	}
	taskGroup.Meta[nomad.ConfigMetaEnvironmentKey] = strconv.Itoa(int(id))
	taskGroup.Meta[nomad.ConfigMetaUsedKey] = nomad.ConfigMetaUnusedValue
	taskGroup.Meta[nomad.ConfigMetaPoolSizeKey] = strconv.Itoa(int(prewarmingPoolSize))
}

func findOrCreateConfigTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	taskGroup := nomad.FindConfigTaskGroup(job)
	if taskGroup == nil {
		taskGroup = nomadApi.NewTaskGroup(nomad.ConfigTaskGroupName, 0)
	}
	createDummyTaskIfNotPresent(taskGroup)
	return taskGroup
}

// createDummyTaskIfNotPresent ensures that a dummy task is in the task group so that the group is accepted by Nomad.
func createDummyTaskIfNotPresent(taskGroup *nomadApi.TaskGroup) {
	var task *nomadApi.Task
	for _, t := range taskGroup.Tasks {
		if t.Name == nomad.DummyTaskName {
			task = t
			break
		}
	}

	if task == nil {
		task = nomadApi.NewTask(nomad.DummyTaskName, nomad.DefaultDummyTaskDriver)
		taskGroup.Tasks = append(taskGroup.Tasks, task)
	}

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["command"] = nomad.DefaultTaskCommand
}
