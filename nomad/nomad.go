package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/url"
	"strings"
)

var log = logging.GetLogger("nomad")

// ExecutorApi provides access to an container orchestration solution
type ExecutorApi interface {
	apiQuerier

	// LoadRunners loads all allocations of the specified job which are running and not about to get stopped.
	LoadRunners(jobId string) (runnerIds []string, err error)

	// MonitorEvaluation monitors the given evaluation ID.
	// It waits until the evaluation reaches one of the states complete, cancelled or failed.
	// If the evaluation was not successful, an error containing the failures is returned.
	// See also https://github.com/hashicorp/nomad/blob/7d5a9ecde95c18da94c9b6ace2565afbfdd6a40d/command/monitor.go#L175
	MonitorEvaluation(evalID string, ctx context.Context) error
}

// ApiClient implements the ExecutorApi interface and can be used to perform different operations on the real Executor API and its return values.
type ApiClient struct {
	apiQuerier
}

// NewExecutorApi creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorApi(nomadURL *url.URL, nomadNamespace string) (ExecutorApi, error) {
	client := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := client.init(nomadURL, nomadNamespace)
	return client, err
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (a *ApiClient) init(nomadURL *url.URL, nomadNamespace string) (err error) {
	err = a.apiQuerier.init(nomadURL, nomadNamespace)
	if err != nil {
		return err
	}
	return nil
}

// LoadRunners loads the allocations of the specified job.
func (a *ApiClient) LoadRunners(jobId string) (runnerIds []string, err error) {
	//list, _, err := apiClient.client.Jobs().Allocations(jobId, true, nil)
	list, err := a.loadRunners(jobId)
	if err != nil {
		return nil, err
	}
	for _, stub := range list {
		// only add allocations which are running and not about to be stopped
		if stub.ClientStatus == nomadApi.AllocClientStatusRunning && stub.DesiredStatus == nomadApi.AllocDesiredStatusRun {
			runnerIds = append(runnerIds, stub.ID)
		}
	}
	return
}

func (a *ApiClient) MonitorEvaluation(evalID string, ctx context.Context) error {
	stream, err := a.EvaluationStream(evalID, ctx)
	if err != nil {
		return err
	}
	// If ctx is cancelled, the stream will be closed by Nomad and we exit the for loop.
	for events := range stream {
		if events.IsHeartbeat() {
			continue
		}
		if err := events.Err; err != nil {
			log.WithError(err).Warn("Error monitoring evaluation")
			return err
		}
		for _, event := range events.Events {
			eval, err := event.Evaluation()
			if err != nil {
				log.WithError(err).Warn("Error retrieving evaluation from streamed event")
				return err
			}
			switch eval.Status {
			case structs.EvalStatusComplete, structs.EvalStatusCancelled, structs.EvalStatusFailed:
				return checkEvaluation(eval)
			default:
			}
		}
	}
	return nil
}

// checkEvaluation checks whether the given evaluation failed.
// If the evaluation failed, it returns an error with a message containing the failure information.
func checkEvaluation(eval *nomadApi.Evaluation) error {
	if len(eval.FailedTGAllocs) == 0 {
		if eval.Status == structs.EvalStatusComplete {
			return nil
		}
		return fmt.Errorf("evaluation could not complete: %q", eval.Status)
	} else {
		messages := []string{
			fmt.Sprintf("Evaluation %q finished with status %q but failed to place all allocations.", eval.ID, eval.Status),
		}
		for tg, metrics := range eval.FailedTGAllocs {
			messages = append(messages, fmt.Sprintf("%s: %#v", tg, metrics))
		}
		if eval.BlockedEval != "" {
			messages = append(messages, fmt.Sprintf("Evaluation %q waiting for additional capacity to place remainder", eval.BlockedEval))
		}
		return errors.New(strings.Join(messages, "\n"))
	}
}
