package nomad

import (
	"context"
	nomadApi "github.com/hashicorp/nomad/api"
	"io"
	"net/url"
)

// apiQuerier provides access to the Nomad functionality.
type apiQuerier interface {
	// init prepares an apiClient to be able to communicate to a provided Nomad API.
	init(nomadURL *url.URL, nomadNamespace string) (err error)

	// LoadJobList loads the list of jobs from the Nomad API.
	LoadJobList() (list []*nomadApi.JobListStub, err error)

	// JobScale returns the scale of the passed job.
	JobScale(jobId string) (jobScale uint, err error)

	// SetJobScale sets the scaling count of the passed job to Nomad.
	SetJobScale(jobId string, count uint, reason string) (err error)

	// DeleteRunner deletes the runner with the given Id.
	DeleteRunner(runnerId string) (err error)

	// ExecuteCommand runs a command in the passed allocation.
	ExecuteCommand(allocationID string, ctx context.Context, command []string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// loadRunners loads all allocations of the specified job.
	loadRunners(jobId string) (allocationListStub []*nomadApi.AllocationListStub, err error)

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
	client       *nomadApi.Client
	namespace    string
	queryOptions *nomadApi.QueryOptions
}

func (nc *nomadAPIClient) init(nomadURL *url.URL, nomadNamespace string) (err error) {
	nc.client, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   nomadURL.String(),
		TLSConfig: &nomadApi.TLSConfig{},
		Namespace: nomadNamespace,
	})
	nc.namespace = nomadNamespace
	nc.queryOptions = &nomadApi.QueryOptions{
		Namespace: nc.namespace,
	}
	return err
}

func (nc *nomadAPIClient) DeleteRunner(runnerID string) (err error) {
	allocation, _, err := nc.client.Allocations().Info(runnerID, nc.queryOptions)
	if err != nil {
		return
	}
	_, err = nc.client.Allocations().Stop(allocation, nil)
	return err
}

func (nc *nomadAPIClient) ExecuteCommand(allocationID string,
	ctx context.Context, command []string, tty bool,
	stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	allocation, _, err := nc.client.Allocations().Info(allocationID, nil)
	if err != nil {
		return 1, err
	}
	return nc.client.Allocations().Exec(ctx, allocation, TaskName, tty, command, stdin, stdout, stderr, nil, nil)
}

func (nc *nomadAPIClient) loadRunners(jobID string) (allocationListStub []*nomadApi.AllocationListStub, err error) {
	allocationListStub, _, err = nc.client.Jobs().Allocations(jobID, true, nc.queryOptions)
	return
}

func (nc *nomadAPIClient) RegisterNomadJob(job *nomadApi.Job) (string, error) {
	job.Namespace = &nc.namespace
	resp, _, err := nc.client.Jobs().Register(job, nil)
	if err != nil {
		return "", err
	}
	if resp.Warnings != "" {
		log.
			WithField("job", job).
			WithField("warnings", resp.Warnings).
			Warn("Received warnings when registering job")
	}
	return resp.EvalID, nil
}

func (nc *nomadAPIClient) EvaluationStream(evalID string, ctx context.Context) (stream <-chan *nomadApi.Events, err error) {
	stream, err = nc.client.EventStream().Stream(
		ctx,
		map[nomadApi.Topic][]string{
			nomadApi.TopicEvaluation: {evalID},
		},
		0,
		nc.queryOptions)
	return
}

func (nc *nomadAPIClient) AllocationStream(ctx context.Context) (stream <-chan *nomadApi.Events, err error) {
	stream, err = nc.client.EventStream().Stream(
		ctx,
		map[nomadApi.Topic][]string{
			nomadApi.TopicAllocation: {},
		},
		0,
		nc.queryOptions)
	return
}
