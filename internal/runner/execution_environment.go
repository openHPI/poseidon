package runner

import (
	"encoding/json"
	"strconv"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/storage"
)

// ExecutionEnvironment are groups of runner that share the configuration stored in the environment.
type ExecutionEnvironment interface {
	json.Marshaler

	// ID returns the id of the environment.
	ID() dto.EnvironmentID
	SetID(id dto.EnvironmentID)
	// PrewarmingPoolSize sets the number of idle runner of this environment that should be prewarmed.
	PrewarmingPoolSize() uint
	SetPrewarmingPoolSize(count uint)
	// ApplyPrewarmingPoolSize creates idle runners according to the PrewarmingPoolSize.
	ApplyPrewarmingPoolSize() error
	// CPULimit sets the share of cpu that a runner should receive at minimum.
	CPULimit() uint
	SetCPULimit(limit uint)
	// MemoryLimit sets the amount of memory that should be available for each runner.
	MemoryLimit() uint
	SetMemoryLimit(limit uint)
	// Image sets the image of the runner, e.g. Docker image.
	Image() string
	SetImage(image string)
	// NetworkAccess sets if a runner should have network access and if ports should be mapped.
	NetworkAccess() (bool, []uint16)
	SetNetworkAccess(allow bool, ports []uint16)
	// SetConfigFrom copies all above attributes from the passed environment to the object itself.
	SetConfigFrom(environment ExecutionEnvironment)

	// Register saves this environment at the executor.
	Register() error
	// Delete removes this environment and all it's runner from the executor and Poseidon itself.
	// Iff local the environment is just removed from Poseidon without external escalation.
	Delete(reason DestroyReason) error

	// Sample returns and removes an arbitrary available runner.
	// ok is true iff a runner was returned.
	Sample() (r Runner, ok bool)
	// AddRunner adds an existing runner to the idle runners of the environment.
	AddRunner(r Runner)
	// DeleteRunner removes an idle runner from the environment and returns it.
	// This function handles only the environment. The runner has to be destroyed separately.
	// ok is true iff the runner was found (and deleted).
	DeleteRunner(id string) (r Runner, ok bool)
	// IdleRunnerCount returns the number of idle runners of the environment.
	IdleRunnerCount() uint
}

// monitorEnvironmentData passes the configuration of the environment e into the monitoring Point p.
func monitorEnvironmentData(p *write.Point, e ExecutionEnvironment, eventType storage.EventType) {
	if eventType == storage.Creation && e != nil {
		p.AddTag("image", e.Image())
		p.AddTag("cpu_limit", strconv.Itoa(int(e.CPULimit())))
		p.AddTag("memory_limit", strconv.Itoa(int(e.MemoryLimit())))
		hasNetworkAccess, _ := e.NetworkAccess()
		p.AddTag("network_access", strconv.FormatBool(hasNetworkAccess))
	}
}
