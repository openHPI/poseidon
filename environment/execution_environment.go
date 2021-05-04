package environment

import (
	"errors"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment/pool"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"time"
)

var log = logging.GetLogger("execution_environment")

// ExecutionEnvironment is a partial image of an execution environment in CodeOcean.
type ExecutionEnvironment interface {
	// NextRunner gets the next available runner and marks it as running.
	// If no runner is available it throws a error after a small timeout
	NextRunner() (runner.Runner, error)
	// Refresh fetches the runners for this execution environment and sends them to the pool.
	// This function does not terminate. Instead it fetches new runners periodically.
	Refresh()
}

// NomadExecutionEnvironment is an implementation that returns a nomad specific execution environment.
// Here it is mapped on a Job in Nomad.
// The jobId has to match the only Nomad task group name!
type NomadExecutionEnvironment struct {
	id               int
	jobId            string
	availableRunners chan runner.Runner
	allRunners       pool.RunnerPool
	nomadApiClient   nomad.ExecutorApi
}

var executionEnvironment ExecutionEnvironment

// DebugInit initializes one execution environment so that its runners can be provided.
// ToDo: This should be replaced by a create Execution Environment route
func DebugInit(runnersPool pool.RunnerPool, nomadApi nomad.ExecutorApi) {
	executionEnvironment = &NomadExecutionEnvironment{
		id:               0,
		jobId:            "python",
		availableRunners: make(chan runner.Runner, 50),
		nomadApiClient:   nomadApi,
		allRunners:       runnersPool,
	}
	go executionEnvironment.Refresh()
}

// GetExecutionEnvironment returns a previously added ExecutionEnvironment.
// This way you can access all Runners for that environment,
func GetExecutionEnvironment(id int) (ExecutionEnvironment, error) {
	// TODO: Remove hardcoded execution environment
	return executionEnvironment, nil
}

func (environment *NomadExecutionEnvironment) NextRunner() (r runner.Runner, err error) {
	select {
	case r = <-environment.availableRunners:
		r.SetStatus(runner.StatusRunning)
		return r, nil
	case <-time.After(50 * time.Millisecond):
		return nil, errors.New("no runners available")
	}
}

// Refresh Big ToDo: Improve this function!! State out that it also rescales the job; Provide context to be terminable...
func (environment *NomadExecutionEnvironment) Refresh() {
	for {
		runners, err := environment.nomadApiClient.LoadRunners(environment.jobId)
		if err != nil {
			log.WithError(err).Printf("Failed fetching runners")
			break
		}
		for _, r := range environment.unusedRunners(runners) {
			// ToDo: Listen on Nomad event stream
			log.Printf("Adding allocation %+v", r)
			environment.allRunners.AddRunner(r)
			environment.availableRunners <- r
		}
		jobScale, err := environment.nomadApiClient.GetJobScale(environment.jobId)
		if err != nil {
			log.WithError(err).Printf("Failed get allocation count")
			break
		}
		neededRunners := cap(environment.availableRunners) - len(environment.availableRunners) + 1
		runnerCount := jobScale + neededRunners
		time.Sleep(50 * time.Millisecond)
		log.Printf("Set job scaling %d", runnerCount)
		err = environment.nomadApiClient.SetJobScaling(environment.jobId, runnerCount, "Runner Requested")
		if err != nil {
			log.WithError(err).Printf("Failed set allocation scaling")
			continue
		}
	}
}

func (environment *NomadExecutionEnvironment) unusedRunners(fetchedRunnerIds []string) (newRunners []runner.Runner) {
	newRunners = make([]runner.Runner, 0)
	for _, runnerId := range fetchedRunnerIds {
		_, ok := environment.allRunners.GetRunner(runnerId)
		if !ok {
			newRunners = append(newRunners, runner.NewExerciseRunner(runnerId))
		}
	}
	return
}
