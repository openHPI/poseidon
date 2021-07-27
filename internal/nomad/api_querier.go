package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/config"
	"io"
)

var (
	ErrorNoAllocationFound = errors.New("no allocation found")
)

// apiQuerier provides access to the Nomad functionality.
type apiQuerier interface {
	// init prepares an apiClient to be able to communicate to a provided Nomad API.
	init(nomadConfig *config.Nomad) (err error)

	// LoadJobList loads the list of jobs from the Nomad API.
	LoadJobList() (list []*nomadApi.JobListStub, err error)

	// JobScale returns the scale of the passed job.
	JobScale(jobID string) (jobScale uint, err error)

	// SetJobScale sets the scaling count of the passed job to Nomad.
	SetJobScale(jobID string, count uint, reason string) (err error)

	// DeleteRunner deletes the runner with the given ID.
	DeleteRunner(runnerID string) (err error)

	// Execute runs a command in the passed job.
	Execute(jobID string, ctx context.Context, command []string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// listJobs loads all jobs with the specified prefix.
	listJobs(prefix string) (allocationListStub []*nomadApi.JobListStub, err error)

	// job returns the job of the given jobID.
	job(jobID string) (job *nomadApi.Job, err error)

	// allocation returns the first allocation of the given job.
	allocation(jobID string) (*nomadApi.Allocation, error)

	// RegisterNomadJob registers a job with Nomad.
	// It returns the evaluation ID that can be used when listening to the Nomad event stream.
	RegisterNomadJob(job *nomadApi.Job) (string, error)

	// EvaluationStream returns a Nomad event stream filtered to return only events belonging to the
	// given evaluation ID.
	EvaluationStream(evalID string, ctx context.Context) (<-chan *nomadApi.Events, error)

	// AllocationStream returns a Nomad event stream filtered to return only allocation events.
	AllocationStream(ctx context.Context) (<-chan *nomadApi.Events, error)
}

// nomadAPIClient implements the nomadApiQuerier interface and provides access to a real Nomad API.
type nomadAPIClient struct {
	client    *nomadApi.Client
	namespace string
}

func (nc *nomadAPIClient) init(nomadConfig *config.Nomad) (err error) {
	nomadTLSConfig := &nomadApi.TLSConfig{}
	if nomadConfig.TLS.Active {
		nomadTLSConfig.CACert = nomadConfig.TLS.CAFile
		nomadTLSConfig.ClientCert = nomadConfig.TLS.CertFile
		nomadTLSConfig.ClientKey = nomadConfig.TLS.KeyFile
	}

	nc.client, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   nomadConfig.URL().String(),
		TLSConfig: nomadTLSConfig,
		Namespace: nomadConfig.Namespace,
		SecretID:  nomadConfig.Token,
	})
	if err != nil {
		return fmt.Errorf("error creating new Nomad client: %w", err)
	}
	nc.namespace = nomadConfig.Namespace
	return nil
}

func (nc *nomadAPIClient) DeleteRunner(runnerID string) (err error) {
	_, _, err = nc.client.Jobs().Deregister(runnerID, true, nc.writeOptions())
	return
}

func (nc *nomadAPIClient) Execute(runnerID string,
	ctx context.Context, command []string, tty bool,
	stdin io.Reader, stdout, stderr io.Writer,
) (int, error) {
	allocations, _, err := nc.client.Jobs().Allocations(runnerID, false, nil)
	if err != nil {
		return 1, fmt.Errorf("error retrieving allocations for runner: %w", err)
	}
	if len(allocations) == 0 {
		return 1, ErrorNoAllocationFound
	}
	allocation, _, err := nc.client.Allocations().Info(allocations[0].ID, nil)
	if err != nil {
		return 1, fmt.Errorf("error retrieving allocation info: %w", err)
	}
	exitCode, err := nc.client.Allocations().Exec(ctx, allocation, TaskName, tty, command, stdin, stdout, stderr, nil, nil)
	if err != nil {
		return 1, fmt.Errorf("error executing command in allocation: %w", err)
	}
	return exitCode, nil
}

func (nc *nomadAPIClient) listJobs(prefix string) ([]*nomadApi.JobListStub, error) {
	q := nomadApi.QueryOptions{
		Namespace: nc.namespace,
		Prefix:    prefix,
	}
	jobs, _, err := nc.client.Jobs().List(&q)
	if err != nil {
		return nil, fmt.Errorf("error listing Nomad jobs: %w", err)
	}
	return jobs, nil
}

func (nc *nomadAPIClient) RegisterNomadJob(job *nomadApi.Job) (string, error) {
	job.Namespace = &nc.namespace
	resp, _, err := nc.client.Jobs().Register(job, nil)
	if err != nil {
		return "", fmt.Errorf("error registering Nomad job: %w", err)
	}
	if resp.Warnings != "" {
		log.
			WithField("job", job).
			WithField("warnings", resp.Warnings).
			Warn("Received warnings when registering job")
	}
	return resp.EvalID, nil
}

func (nc *nomadAPIClient) EvaluationStream(evalID string, ctx context.Context) (<-chan *nomadApi.Events, error) {
	stream, err := nc.client.EventStream().Stream(
		ctx,
		map[nomadApi.Topic][]string{
			nomadApi.TopicEvaluation: {evalID},
		},
		0,
		nc.queryOptions())
	if err != nil {
		return nil, fmt.Errorf("error retrieving Nomad Evaluation event stream: %w", err)
	}
	return stream, nil
}

func (nc *nomadAPIClient) AllocationStream(ctx context.Context) (<-chan *nomadApi.Events, error) {
	stream, err := nc.client.EventStream().Stream(
		ctx,
		map[nomadApi.Topic][]string{
			nomadApi.TopicAllocation: {
				// Necessary to have the "topic" URL param show up in the HTTP request to Nomad.
				// Without the param, Nomad will try to deliver all event types.
				// However, this includes some events that require a management authentication token.
				// As Poseidon uses no such token, the request will return a permission denied error.
				"*",
			},
		},
		0,
		nc.queryOptions())
	if err != nil {
		return nil, fmt.Errorf("error retrieving Nomad Allocation event stream: %w", err)
	}
	return stream, nil
}

func (nc *nomadAPIClient) queryOptions() *nomadApi.QueryOptions {
	return &nomadApi.QueryOptions{
		Namespace: nc.namespace,
	}
}

func (nc *nomadAPIClient) writeOptions() *nomadApi.WriteOptions {
	return &nomadApi.WriteOptions{
		Namespace: nc.namespace,
	}
}

// LoadJobList loads the list of jobs from the Nomad api.
func (nc *nomadAPIClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nc.queryOptions())
	return
}

// JobScale returns the scale of the passed job.
func (nc *nomadAPIClient) JobScale(jobID string) (uint, error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobID, nc.queryOptions())
	if err != nil {
		return 0, fmt.Errorf("error retrieving scale status of job: %w", err)
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale := uint(status.TaskGroups[TaskGroupName].Running)
	return jobScale, nil
}

// SetJobScale sets the scaling count of the passed job to Nomad.
func (nc *nomadAPIClient) SetJobScale(jobID string, count uint, reason string) (err error) {
	intCount := int(count)
	_, _, err = nc.client.Jobs().Scale(jobID, TaskGroupName, &intCount, reason, false, nil, nil)
	return
}

func (nc *nomadAPIClient) job(jobID string) (job *nomadApi.Job, err error) {
	job, _, err = nc.client.Jobs().Info(jobID, nil)
	return
}

func (nc *nomadAPIClient) allocation(jobID string) (alloc *nomadApi.Allocation, err error) {
	allocs, _, err := nc.client.Jobs().Allocations(jobID, false, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting Nomad job allocations: %w", err)
	}
	if len(allocs) == 0 {
		return nil, ErrorNoAllocationFound
	}
	alloc, _, err = nc.client.Allocations().Info(allocs[0].ID, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting Nomad allocation info: %w", err)
	}
	return alloc, nil
}
