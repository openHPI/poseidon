package environment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"strconv"
)

const (
	portNumberBase = 10
)

var (
	ErrorUpdatingExecutionEnvironment = errors.New("errors occurred when updating environment")
)

type NomadEnvironment struct {
	jobHCL      string
	job         *nomadApi.Job
	idleRunners runner.Storage
}

func NewNomadEnvironment(jobHCL string) (*NomadEnvironment, error) {
	job, err := parseJob(jobHCL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Nomad job: %w", err)
	}

	return &NomadEnvironment{jobHCL, job, runner.NewLocalRunnerStorage()}, nil
}

func (n *NomadEnvironment) ID() dto.EnvironmentID {
	id, err := nomad.EnvironmentIDFromTemplateJobID(*n.job.ID)
	if err != nil {
		log.WithError(err).Error("Environment ID can not be parsed from Job")
	}
	return id
}

func (n *NomadEnvironment) SetID(id dto.EnvironmentID) {
	name := nomad.TemplateJobID(id)
	n.job.ID = &name
	n.job.Name = &name
}

func (n *NomadEnvironment) PrewarmingPoolSize() uint {
	configTaskGroup := nomad.FindOrCreateConfigTaskGroup(n.job)
	count, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaPoolSizeKey])
	if err != nil {
		log.WithError(err).Error("Prewarming pool size can not be parsed from Job")
	}
	return uint(count)
}

func (n *NomadEnvironment) SetPrewarmingPoolSize(count uint) {
	taskGroup := nomad.FindOrCreateConfigTaskGroup(n.job)

	if taskGroup.Meta == nil {
		taskGroup.Meta = make(map[string]string)
	}
	taskGroup.Meta[nomad.ConfigMetaPoolSizeKey] = strconv.Itoa(int(count))
}

func (n *NomadEnvironment) CPULimit() uint {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)
	return uint(*defaultTask.Resources.CPU)
}

func (n *NomadEnvironment) SetCPULimit(limit uint) {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)

	integerCPULimit := int(limit)
	defaultTask.Resources.CPU = &integerCPULimit
}

func (n *NomadEnvironment) MemoryLimit() uint {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)
	return uint(*defaultTask.Resources.MemoryMB)
}

func (n *NomadEnvironment) SetMemoryLimit(limit uint) {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)

	integerMemoryLimit := int(limit)
	defaultTask.Resources.MemoryMB = &integerMemoryLimit
}

func (n *NomadEnvironment) Image() string {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)
	image, ok := defaultTask.Config["image"].(string)
	if !ok {
		image = ""
	}
	return image
}

func (n *NomadEnvironment) SetImage(image string) {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)
	defaultTask.Config["image"] = image
}

func (n *NomadEnvironment) NetworkAccess() (allowed bool, ports []uint16) {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)

	allowed = defaultTask.Config["network_mode"] != "none"
	if len(defaultTaskGroup.Networks) > 0 {
		networkResource := defaultTaskGroup.Networks[0]
		for _, port := range networkResource.DynamicPorts {
			ports = append(ports, uint16(port.To))
		}
	}
	return allowed, ports
}

func (n *NomadEnvironment) SetNetworkAccess(allow bool, exposedPorts []uint16) {
	defaultTaskGroup := nomad.FindOrCreateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindOrCreateDefaultTask(defaultTaskGroup)

	if len(defaultTaskGroup.Tasks) == 0 {
		// This function is only used internally and must be called as last step when configuring the task.
		// This error is not recoverable.
		log.Fatal("Can't configure network before task has been configured!")
	}

	if allow {
		var networkResource *nomadApi.NetworkResource
		if len(defaultTaskGroup.Networks) == 0 {
			networkResource = &nomadApi.NetworkResource{}
			defaultTaskGroup.Networks = []*nomadApi.NetworkResource{networkResource}
		} else {
			networkResource = defaultTaskGroup.Networks[0]
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
		defaultTask.Config["network_mode"] = ""
	} else {
		// Somehow, we can't set the network mode to none in the NetworkResource on task group level.
		// See https://github.com/hashicorp/nomad/issues/10540
		defaultTask.Config["network_mode"] = "none"
		// Explicitly set Networks to signal Nomad to remove the possibly existing networkResource
		defaultTaskGroup.Networks = []*nomadApi.NetworkResource{}
	}
}

// Register creates a Nomad job based on the default job configuration and the given parameters.
// It registers the job with Nomad and waits until the registration completes.
func (n *NomadEnvironment) Register(apiClient nomad.ExecutorAPI) error {
	evalID, err := apiClient.RegisterNomadJob(n.job)
	if err != nil {
		return fmt.Errorf("couldn't register job: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), nomad.RegisterTimeout)
	defer cancel()
	err = apiClient.MonitorEvaluation(evalID, ctx)
	if err != nil {
		return fmt.Errorf("error during the monitoring of the environment job: %w", err)
	}
	return nil
}

func (n *NomadEnvironment) Delete(apiClient nomad.ExecutorAPI) error {
	err := n.removeRunners(apiClient, uint(n.idleRunners.Length()))
	if err != nil {
		return err
	}
	err = apiClient.DeleteJob(*n.job.ID)
	if err != nil {
		return fmt.Errorf("couldn't delete environment job: %w", err)
	}
	return nil
}

func (n *NomadEnvironment) Scale(apiClient nomad.ExecutorAPI, forcePull bool) error {
	required := int(n.PrewarmingPoolSize()) - n.idleRunners.Length()

	if required > 0 {
		return n.createRunners(apiClient, uint(required), forcePull)
	} else {
		return n.removeRunners(apiClient, uint(-required))
	}
}

func (n *NomadEnvironment) UpdateRunnerSpecs(apiClient nomad.ExecutorAPI, forcePull bool) error {
	runners, err := apiClient.LoadRunnerIDs(n.ID().ToString())
	if err != nil {
		return fmt.Errorf("update environment couldn't load runners: %w", err)
	}

	var occurredError error
	for _, id := range runners {
		// avoid taking the address of the loop variable
		runnerID := id
		updatedRunnerJob := n.DeepCopyJob()
		updatedRunnerJob.ID = &runnerID
		updatedRunnerJob.Name = &runnerID
		nomad.SetForcePullFlag(updatedRunnerJob, forcePull)

		err := apiClient.RegisterRunnerJob(updatedRunnerJob)
		if err != nil {
			if occurredError == nil {
				occurredError = ErrorUpdatingExecutionEnvironment
			}
			occurredError = fmt.Errorf("%w; new api error for runner %s - %v", occurredError, id, err)
		}
	}
	return occurredError
}

func (n *NomadEnvironment) Sample(apiClient nomad.ExecutorAPI) (runner.Runner, bool) {
	r, ok := n.idleRunners.Sample()
	if ok {
		go func() {
			err := n.createRunner(apiClient, false)
			if err != nil {
				log.WithError(err).WithField("environmentID", n.ID()).Error("Couldn't create new runner for claimed one")
			}
		}()
	}
	return r, ok
}

func (n *NomadEnvironment) AddRunner(r runner.Runner) {
	n.idleRunners.Add(r)
}

func (n *NomadEnvironment) DeleteRunner(id string) {
	n.idleRunners.Delete(id)
}

func (n *NomadEnvironment) IdleRunnerCount() int {
	return n.idleRunners.Length()
}

// MarshalJSON implements the json.Marshaler interface.
// This converts the NomadEnvironment into the expected schema for dto.ExecutionEnvironmentData.
func (n *NomadEnvironment) MarshalJSON() (res []byte, err error) {
	networkAccess, exposedPorts := n.NetworkAccess()

	res, err = json.Marshal(dto.ExecutionEnvironmentData{
		ID: int(n.ID()),
		ExecutionEnvironmentRequest: dto.ExecutionEnvironmentRequest{
			PrewarmingPoolSize: n.PrewarmingPoolSize(),
			CPULimit:           n.CPULimit(),
			MemoryLimit:        n.MemoryLimit(),
			Image:              n.Image(),
			NetworkAccess:      networkAccess,
			ExposedPorts:       exposedPorts,
		},
	})
	if err != nil {
		return res, fmt.Errorf("couldn't marshal execution environment: %w", err)
	}
	return res, nil
}

// DeepCopyJob clones the native Nomad job in a way that it can be used as Runner job.
func (n *NomadEnvironment) DeepCopyJob() *nomadApi.Job {
	copyJob, err := parseJob(n.jobHCL)
	if err != nil {
		log.WithError(err).Error("The HCL of an existing environment should throw no error!")
		return nil
	}
	copyEnvironment := &NomadEnvironment{job: copyJob}

	copyEnvironment.SetConfigFrom(n)
	return copyEnvironment.job
}

func (n *NomadEnvironment) SetConfigFrom(environment runner.ExecutionEnvironment) {
	n.SetID(environment.ID())
	n.SetPrewarmingPoolSize(environment.PrewarmingPoolSize())
	n.SetCPULimit(environment.CPULimit())
	n.SetMemoryLimit(environment.MemoryLimit())
	n.SetImage(environment.Image())
	n.SetNetworkAccess(environment.NetworkAccess())
}

func parseJob(jobHCL string) (*nomadApi.Job, error) {
	config := jobspec2.ParseConfig{
		Body:    []byte(jobHCL),
		AllowFS: false,
		Strict:  true,
	}
	job, err := jobspec2.ParseWithConfig(&config)
	if err != nil {
		return job, fmt.Errorf("couldn't parse job HCL: %w", err)
	}
	return job, nil
}

func (n *NomadEnvironment) createRunners(apiClient nomad.ExecutorAPI, count uint, forcePull bool) error {
	log.WithField("runnersRequired", count).WithField("id", n.ID()).Debug("Creating new runners")
	for i := 0; i < int(count); i++ {
		err := n.createRunner(apiClient, forcePull)
		if err != nil {
			return fmt.Errorf("couldn't create new runner: %w", err)
		}
	}
	return nil
}

func (n *NomadEnvironment) createRunner(apiClient nomad.ExecutorAPI, forcePull bool) error {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return fmt.Errorf("failed generating runner id: %w", err)
	}

	newRunnerID := nomad.RunnerJobID(n.ID(), newUUID.String())
	template := n.DeepCopyJob()
	template.ID = &newRunnerID
	template.Name = &newRunnerID
	nomad.SetForcePullFlag(template, forcePull)

	err = apiClient.RegisterRunnerJob(template)
	if err != nil {
		return fmt.Errorf("error registering new runner job: %w", err)
	}
	return nil
}

func (n *NomadEnvironment) removeRunners(apiClient nomad.ExecutorAPI, count uint) error {
	log.WithField("runnersToDelete", count).WithField("id", n.ID()).Debug("Removing idle runners")
	for i := 0; i < int(count); i++ {
		r, ok := n.idleRunners.Sample()
		if !ok {
			return fmt.Errorf("could not delete expected idle runner: %w", runner.ErrRunnerNotFound)
		}
		err := apiClient.DeleteJob(r.ID())
		if err != nil {
			return fmt.Errorf("could not delete expected Nomad idle runner: %w", err)
		}
	}
	return nil
}
