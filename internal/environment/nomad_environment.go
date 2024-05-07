package environment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
)

const portNumberBase = 10

var ErrScaleDown = errors.New("cannot scale down the environment")

type NomadEnvironment struct {
	apiClient   nomad.ExecutorAPI
	jobHCL      string
	job         *nomadApi.Job
	idleRunners storage.Storage[runner.Runner]
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewNomadEnvironment(environmentID dto.EnvironmentID, apiClient nomad.ExecutorAPI, jobHCL string) (*NomadEnvironment, error) {
	job, err := parseJob(jobHCL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Nomad job: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	e := &NomadEnvironment{apiClient, jobHCL, job, nil, ctx, cancel}
	e.idleRunners = storage.NewMonitoredLocalStorage[runner.Runner](monitoring.MeasurementIdleRunnerNomad,
		runner.MonitorEnvironmentID[runner.Runner](environmentID), time.Minute, ctx)
	return e, nil
}

func NewNomadEnvironmentFromRequest(
	apiClient nomad.ExecutorAPI, jobHCL string, environmentID dto.EnvironmentID, request dto.ExecutionEnvironmentRequest) (
	*NomadEnvironment, error) {
	environment, err := NewNomadEnvironment(environmentID, apiClient, jobHCL)
	if err != nil {
		return nil, err
	}
	environment.SetID(environmentID)

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
	monitoring.ChangedPrewarmingPoolSize(n.ID(), count)
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
	maxMemoryLimit := defaultTask.Resources.MemoryMaxMB
	if maxMemoryLimit != nil {
		return uint(*maxMemoryLimit)
	} else {
		return 0
	}
}

func (n *NomadEnvironment) SetMemoryLimit(limit uint) {
	defaultTaskGroup := nomad.FindAndValidateDefaultTaskGroup(n.job)
	defaultTask := nomad.FindAndValidateDefaultTask(defaultTaskGroup)

	integerMemoryMaxLimit := int(limit)
	defaultTask.Resources.MemoryMaxMB = &integerMemoryMaxLimit
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
		networkResource := config.Config.Nomad.Network
		for _, portNumber := range exposedPorts {
			port := nomadApi.Port{
				Label: strconv.FormatUint(uint64(portNumber), portNumberBase),
				To:    int(portNumber),
			}
			networkResource.DynamicPorts = append(networkResource.DynamicPorts, port)
		}
		if len(defaultTaskGroup.Networks) == 0 {
			defaultTaskGroup.Networks = []*nomadApi.NetworkResource{&networkResource}
		} else {
			defaultTaskGroup.Networks[0] = &networkResource
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

func (n *NomadEnvironment) Delete(reason runner.DestroyReason) error {
	n.cancel()

	err := n.removeRunners(reason)
	if err != nil {
		return err
	}

	if !errors.Is(reason, runner.ErrLocalDestruction) {
		err = n.apiClient.DeleteJob(*n.job.ID)
		if err != nil {
			return fmt.Errorf("couldn't delete environment job: %w", err)
		}
	}
	return nil
}

func (n *NomadEnvironment) ApplyPrewarmingPoolSize() error {
	required := int(n.PrewarmingPoolSize()) - int(n.idleRunners.Length())

	if required < 0 {
		log.WithError(ErrScaleDown).
			WithField(dto.KeyEnvironmentID, n.ID().ToString()).
			WithField("offset", -required).Info("Too many idle runner")
		return nil
	}
	return n.createRunners(uint(required), true)
}

func (n *NomadEnvironment) Sample() (runner.Runner, bool) {
	r, ok := n.idleRunners.Sample()
	if ok && n.idleRunners.Length() < n.PrewarmingPoolSize() {
		go func() {
			err := util.RetryExponentialWithContext(n.ctx, func() error { return n.createRunner(false) })
			if err != nil {
				log.WithError(err).WithField(dto.KeyEnvironmentID, n.ID().ToString()).
					Error("Couldn't create new runner for claimed one")
			}
		}()
	} else if ok {
		log.WithField(dto.KeyEnvironmentID, n.ID().ToString()).Info("Too many idle runner")
	}
	return r, ok
}

func (n *NomadEnvironment) AddRunner(object runner.Runner) {
	if replacedRunner, ok := n.idleRunners.Get(object.ID()); ok {
		err := replacedRunner.Destroy(runner.ErrDestroyedAndReplaced)
		if err != nil {
			log.WithError(err).Warn("failed removing runner before replacing it")
		}
	}
	n.idleRunners.Add(object.ID(), object)
}

func (n *NomadEnvironment) DeleteRunner(id string) (r runner.Runner, ok bool) {
	r, ok = n.idleRunners.Get(id)
	n.idleRunners.Delete(id)
	return r, ok
}

func (n *NomadEnvironment) IdleRunnerCount() uint {
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
	jobConfig := jobspec2.ParseConfig{
		Body:    []byte(jobHCL),
		AllowFS: false,
		Strict:  true,
	}
	job, err := jobspec2.ParseWithConfig(&jobConfig)
	if err != nil {
		return job, fmt.Errorf("couldn't parse job HCL: %w", err)
	}
	return job, nil
}

func (n *NomadEnvironment) createRunners(count uint, forcePull bool) error {
	log.WithField("runnersRequired", count).WithField(dto.KeyEnvironmentID, n.ID()).Debug("Creating new runners")
	for range count {
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
func (n *NomadEnvironment) removeRunners(reason runner.DestroyReason) error {
	// This prevents a race condition where the number of required runners is miscalculated in the up-scaling process
	// based on the number of allocation that has been stopped at the moment of the scaling.
	for _, r := range n.idleRunners.List() {
		n.idleRunners.Delete(r.ID())
		if err := r.Destroy(runner.ErrLocalDestruction); err != nil {
			log.WithError(err).Warn("failed to remove runner locally")
		}
	}

	if errors.Is(reason, runner.ErrLocalDestruction) {
		return nil
	}

	ids, err := n.apiClient.LoadRunnerIDs(nomad.RunnerJobID(n.ID(), ""))
	if err != nil {
		return fmt.Errorf("failed to load runner ids: %w", err)
	}

	// Block execution until Nomad confirmed all deletion requests.
	var wg sync.WaitGroup
	for _, runnerID := range ids {
		wg.Add(1)

		go func(jobID string) {
			defer wg.Done()
			deleteErr := n.apiClient.DeleteJob(jobID)
			if deleteErr != nil {
				err = deleteErr
			}
		}(runnerID)
	}
	wg.Wait()
	return err
}
