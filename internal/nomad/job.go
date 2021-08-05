package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"strconv"
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
)

var (
	TaskArgs                     = []string{"infinity"}
	ErrorConfigTaskGroupNotFound = errors.New("config task group not found in job")
)

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

// RegisterTemplateJob creates a Nomad job based on the default job configuration and the given parameters.
// It registers the job with Nomad and waits until the registration completes.
func (a *APIClient) RegisterTemplateJob(
	basisJob *nomadApi.Job,
	id string,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) (*nomadApi.Job, error) {
	job := CreateTemplateJob(basisJob, id, prewarmingPoolSize,
		cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
	evalID, err := a.apiQuerier.RegisterNomadJob(job)
	if err != nil {
		return nil, fmt.Errorf("couldn't register template job: %w", err)
	}
	return job, a.MonitorEvaluation(evalID, context.Background())
}

// CreateTemplateJob creates a Nomad job based on the default job configuration and the given parameters.
// It registers the job with Nomad and waits until the registration completes.
func CreateTemplateJob(
	basisJob *nomadApi.Job,
	id string,
	prewarmingPoolSize, cpuLimit, memoryLimit uint,
	image string,
	networkAccess bool,
	exposedPorts []uint16) *nomadApi.Job {
	job := *basisJob
	job.ID = &id
	job.Name = &id

	var taskGroup = createTaskGroup(&job, TaskGroupName)
	configureTask(taskGroup, TaskName, cpuLimit, memoryLimit, image, networkAccess, exposedPorts)
	storeTemplateConfiguration(&job, prewarmingPoolSize)

	return &job
}

func (a *APIClient) RegisterRunnerJob(template *nomadApi.Job) error {
	storeRunnerConfiguration(template)

	evalID, err := a.apiQuerier.RegisterNomadJob(template)
	if err != nil {
		return fmt.Errorf("couldn't register runner job: %w", err)
	}
	return a.MonitorEvaluation(evalID, context.Background())
}

func createTaskGroup(job *nomadApi.Job, name string) *nomadApi.TaskGroup {
	var taskGroup *nomadApi.TaskGroup
	if len(job.TaskGroups) == 0 {
		taskGroup = nomadApi.NewTaskGroup(name, TaskCount)
		job.TaskGroups = []*nomadApi.TaskGroup{taskGroup}
	} else {
		taskGroup = job.TaskGroups[0]
		taskGroup.Name = &name
		count := TaskCount
		taskGroup.Count = &count
	}
	return taskGroup
}

const portNumberBase = 10

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
				Label: strconv.FormatUint(uint64(portNumber), portNumberBase),
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
		task = nomadApi.NewTask(name, TaskDriver)
		taskGroup.Tasks = []*nomadApi.Task{task}
	} else {
		task = taskGroup.Tasks[0]
		task.Name = name
	}
	integerCPULimit := int(cpuLimit)
	integerMemoryLimit := int(memoryLimit)
	if task.Resources == nil {
		task.Resources = nomadApi.DefaultResources()
	}
	task.Resources.CPU = &integerCPULimit
	task.Resources.MemoryMB = &integerMemoryLimit

	if task.Config == nil {
		task.Config = make(map[string]interface{})
	}
	task.Config["image"] = image
	task.Config["command"] = TaskCommand
	task.Config["args"] = TaskArgs

	configureNetwork(taskGroup, networkAccess, exposedPorts)
}

func storeTemplateConfiguration(job *nomadApi.Job, prewarmingPoolSize uint) {
	taskGroup := findOrCreateConfigTaskGroup(job)

	taskGroup.Meta = make(map[string]string)
	taskGroup.Meta[ConfigMetaPoolSizeKey] = strconv.Itoa(int(prewarmingPoolSize))
}

func storeRunnerConfiguration(job *nomadApi.Job) {
	taskGroup := findOrCreateConfigTaskGroup(job)

	taskGroup.Meta = make(map[string]string)
	taskGroup.Meta[ConfigMetaUsedKey] = ConfigMetaUnusedValue
}

func findOrCreateConfigTaskGroup(job *nomadApi.Job) *nomadApi.TaskGroup {
	taskGroup := FindConfigTaskGroup(job)
	if taskGroup == nil {
		taskGroup = nomadApi.NewTaskGroup(ConfigTaskGroupName, 0)
		job.AddTaskGroup(taskGroup)
	}
	createConfigTaskIfNotPresent(taskGroup)
	return taskGroup
}

// createConfigTaskIfNotPresent ensures that a dummy task is in the task group so that the group is accepted by Nomad.
func createConfigTaskIfNotPresent(taskGroup *nomadApi.TaskGroup) {
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
}
