package nomad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/websocket"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/logging"
)

var (
	ErrNoAllocationFound      = errors.New("no allocation found")
	ErrNomadUnknownAllocation = errors.New("unknown allocation")
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

	// DeleteJob deletes the Job with the given ID.
	DeleteJob(jobID string) (err error)

	// Execute runs a command in the passed job.
	Execute(ctx context.Context, jobID string, command string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// listJobs loads all jobs with the specified prefix.
	listJobs(prefix string) (jobListStub []*nomadApi.JobListStub, err error)

	// job returns the job of the given jobID.
	job(jobID string) (job *nomadApi.Job, err error)

	// listAllocations loads all allocations.
	listAllocations() (allocationListStub []*nomadApi.AllocationListStub, err error)

	// allocation returns the first allocation of the given job.
	allocation(jobID string) (*nomadApi.Allocation, error)

	// RegisterNomadJob registers a job with Nomad.
	// It returns the evaluation ID that can be used when listening to the Nomad event stream.
	RegisterNomadJob(job *nomadApi.Job) (string, error)

	// EventStream returns a Nomad event stream filtered to return only allocation and evaluation events.
	EventStream(ctx context.Context) (<-chan *nomadApi.Events, error)
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

func (nc *nomadAPIClient) DeleteJob(jobID string) (err error) {
	_, _, err = nc.client.Jobs().Deregister(jobID, true, nc.writeOptions())
	return
}

func (nc *nomadAPIClient) Execute(ctx context.Context, runnerID string, cmd string, tty bool,
	stdin io.Reader, stdout, stderr io.Writer,
) (int, error) {
	log.WithContext(ctx).WithField("command", strings.ReplaceAll(cmd, "\n", "")).Trace("Requesting Nomad Exec")
	var allocations []*nomadApi.AllocationListStub
	var err error
	logging.StartSpan(ctx, "nomad.execute.list", "List Allocations for id", func(_ context.Context) {
		allocations, _, err = nc.client.Jobs().Allocations(runnerID, false, nil)
	})
	if err != nil {
		return 1, fmt.Errorf("error retrieving allocations for runner: %w", err)
	}
	if len(allocations) == 0 {
		return 1, ErrNoAllocationFound
	}

	var allocation *nomadApi.Allocation
	logging.StartSpan(ctx, "nomad.execute.info", "List Data of Allocation", func(_ context.Context) {
		allocation, _, err = nc.client.Allocations().Info(allocations[0].ID, nil)
	})
	if err != nil {
		return 1, fmt.Errorf("error retrieving allocation info: %w", err)
	}

	var exitCode int
	span := sentry.StartSpan(ctx, "nomad.execute.exec")
	span.Description = "Execute Command in Allocation"

	debugWriter := NewSentryDebugWriter(span.Context(), stdout)
	commands := []string{"/bin/bash", "-c", cmd}
	exitCode, err = nc.client.Allocations().
		Exec(span.Context(), allocation, TaskName, tty, commands, stdin, debugWriter, stderr, nil, nil)
	debugWriter.Close(exitCode)

	span.SetData("command", cmd)
	span.Finish()

	switch {
	case err == nil:
		return exitCode, nil
	case websocket.IsCloseError(errors.Unwrap(err), websocket.CloseNormalClosure):
		log.WithContext(ctx).WithError(err).Info("The exit code could not be received.")
		return 0, nil
	case errors.Is(err, context.Canceled):
		log.WithContext(ctx).Debug("Execution canceled by context")
		return 0, nil
	case errors.Is(err, io.ErrUnexpectedEOF), strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()):
		log.WithContext(ctx).WithError(err).Warn("Unexpected EOF for Execute")
		return 0, nil
	case strings.Contains(err.Error(), "Unknown allocation"):
		return 1, ErrNomadUnknownAllocation
	default:
		return 1, fmt.Errorf("error executing command in allocation: %w", err)
	}
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

func (nc *nomadAPIClient) EventStream(ctx context.Context) (<-chan *nomadApi.Events, error) {
	stream, err := nc.client.EventStream().Stream(
		ctx,
		map[nomadApi.Topic][]string{
			nomadApi.TopicEvaluation: {"*"},
			nomadApi.TopicAllocation: {
				// Necessary to have the "topic" URL param show up in the HTTP request to Nomad.
				// Without the param, Nomad will try to deliver all event types.
				// However, this includes some events that require a management authentication token.
				// As Poseidon uses no such token, the request will return a permission denied error.
				"*",
			},
			nomadApi.TopicJob: {"*"},
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

func (nc *nomadAPIClient) listAllocations() ([]*nomadApi.AllocationListStub, error) {
	allocationListStubs, _, err := nc.client.Allocations().List(nc.queryOptions())
	if err != nil {
		return nil, fmt.Errorf("error listing Nomad allocations: %w", err)
	}
	return allocationListStubs, nil
}

func (nc *nomadAPIClient) allocation(jobID string) (alloc *nomadApi.Allocation, err error) {
	allocs, _, err := nc.client.Jobs().Allocations(jobID, false, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting Nomad job allocations: %w", err)
	}
	if len(allocs) == 0 {
		return nil, ErrNoAllocationFound
	}
	alloc, _, err = nc.client.Allocations().Info(allocs[0].ID, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting Nomad allocation info: %w", err)
	}
	return alloc, nil
}
