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
	"sync"
)

const (
	portNumberBase = 10
)

var ErrScaleDown = errors.New("cannot scale down the environment")

type NomadEnvironment struct {
	apiClient   nomad.ExecutorAPI
	jobHCL      string
	job         *nomadApi.Job
	idleRunners runner.Storage
}

func NewNomadEnvironment(apiClient nomad.ExecutorAPI, jobHCL string) (*NomadEnvironment, error) {
	job, err := parseJob(jobHCL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Nomad job: %w", err)
	}

	return &NomadEnvironment{apiClient, jobHCL, job, runner.NewLocalRunnerStorage()}, nil
}

func NewNomadEnvironmentFromRequest(
	apiClient nomad.ExecutorAPI, jobHCL string, id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest) (
	*NomadEnvironment, error) {
	environment, err := NewNomadEnvironment(apiClient, jobHCL)
	if err != nil {
		return nil, err
	}
	environment.SetID(id)

	// Set options according to request
	environment.SetPrewarmingPoolSize(request.PrewarmingPoolSize)
	environment.SetCPULimit(request.CPULimit)
	environment.SetMemoryLimit(request.MemoryLimit)
	environment.SetImage(request.Image)
	environment.SetNetworkAccess(request.NetworkAccess, request.ExposedPorts)
	return environment, nil
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
	configTaskGroup := nomad.FindAndValidateConfigTaskGroup(n.job)
	count, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaPoolSizeKey])
	if err != nil {
		log.WithError(err).Error("Prewarming pool size can not be parsed from Job")
	}
	return uint(count)
}

func (n *NomadEnvironment) SetPrewarmingPoolSize(count uint) {
	taskGroup := nomad.FindAndValidateConfigTaskGroup(n.job)

	if taskGroup.Meta == nil {
		taskGroup.Meta = make(map[string]string)
	}
	taskGroup.Meta[nomad.ConfigMetaPoolSizeKey] = strconv.Itoa(int(count))
}

func (n *NomadEnvironment) CPULimit() uint {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)
	return uint(*defaultTask.Resources.CPU)
}

func (n *NomadEnvironment) SetCPULimit(limit uint) {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

	integerCPULimit := int(limit)
	defaultTask.Resources.CPU = &integerCPULimit
}

func (n *NomadEnvironment) MemoryLimit() uint {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)
	return uint(*defaultTask.Resources.MemoryMB)
}

func (n *NomadEnvironment) SetMemoryLimit(limit uint) {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

	integerMemoryLimit := int(limit)
	defaultTask.Resources.MemoryMB = &integerMemoryLimit
}

func (n *NomadEnvironment) Image() string {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)
	image, ok := defaultTask.Config["image"].(string)
	if !ok {
		image = ""
	}
	return image
}

func (n *NomadEnvironment) SetImage(image string) {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)
	defaultTask.Config["image"] = image
}

func (n *NomadEnvironment) NetworkAccess() (allowed bool, ports []uint16) {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

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
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

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
func (n *NomadEnvironment) Register() error {
	nomad.SetForcePullFlag(n.job, true) // This must be the default as otherwise new runners could have different images.
	evalID, err := n.apiClient.RegisterNomadJob(n.job)
	if err != nil {
		return fmt.Errorf("couldn't register job: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), nomad.RegisterTimeout)
	defer cancel()
	err = n.apiClient.MonitorEvaluation(evalID, ctx)
	if err != nil {
		return fmt.Errorf("error during the monitoring of the environment job: %w", err)
	}
	return nil
}

func (n *NomadEnvironment) Delete() error {
	err := n.removeRunners()
	if err != nil {
		return err
	}
	err = n.apiClient.DeleteJob(*n.job.ID)
	if err != nil {
		return fmt.Errorf("couldn't delete environment job: %w", err)
	}
	return nil
}

func (n *NomadEnvironment) ApplyPrewarmingPoolSize() error {
	required := int(n.PrewarmingPoolSize()) - n.idleRunners.Length()

	if required < 0 {
		return fmt.Errorf("%w. Runners to remove: %d", ErrScaleDown, -required)
	}
	return n.createRunners(uint(required), true)
}

func (n *NomadEnvironment) Sample() (runner.Runner, bool) {
	r, ok := n.idleRunners.Sample()
	if ok {
		go func() {
			err := n.createRunner(false)
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

// SetConfigFrom gets the options from the environment job and saves it into another temporary job.
// IMPROVE: The getters use a validation function that theoretically could edit the environment job.
// But this modification might never been saved to Nomad.
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

func (n *NomadEnvironment) createRunners(count uint, forcePull bool) error {
	log.WithField("runnersRequired", count).WithField("id", n.ID()).Debug("Creating new runners")
	for i := 0; i < int(count); i++ {
		err := n.createRunner(forcePull)
		if err != nil {
			return fmt.Errorf("couldn't create new runner: %w", err)
		}
	}
	return nil
}

func (n *NomadEnvironment) createRunner(forcePull bool) error {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return fmt.Errorf("failed generating runner id: %w", err)
	}

	newRunnerID := nomad.RunnerJobID(n.ID(), newUUID.String())
	template := n.DeepCopyJob()
	template.ID = &newRunnerID
	template.Name = &newRunnerID
	nomad.SetForcePullFlag(template, forcePull)

	err = n.apiClient.RegisterRunnerJob(template)
	if err != nil {
		return fmt.Errorf("error registering new runner job: %w", err)
	}
	return nil
}

// removeRunners removes all (idle and used) runners for the given environment n.
func (n *NomadEnvironment) removeRunners() error {
	// This prevents a race condition where the number of required runners is miscalculated in the up-scaling process
	// based on the number of allocation that has been stopped at the moment of the scaling.
	n.idleRunners.Purge()

	ids, err := n.apiClient.LoadRunnerIDs(nomad.RunnerJobID(n.ID(), ""))
	if err != nil {
		return fmt.Errorf("failed to load runner ids: %w", err)
	}

	// Block execution until Nomad confirmed all deletion requests.
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(jobID string) {
			defer wg.Done()
			deleteErr := n.apiClient.DeleteJob(jobID)
			if deleteErr != nil {
				err = deleteErr
			}
		}(id)
	}
	wg.Wait()
	return err
}
