package runner

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/execution"
	"io"
)

// AWSFunctionWorkload is an abstraction to build a request to an AWS Lambda Function.
type AWSFunctionWorkload struct {
	InactivityTimer
	id         string
	fs         map[dto.FilePath][]byte
	executions execution.Storer
	onDestroy  destroyRunnerHandler
}

// NewAWSFunctionWorkload creates a new AWSFunctionWorkload with the provided id.
func NewAWSFunctionWorkload(onDestroy destroyRunnerHandler) (*AWSFunctionWorkload, error) {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("failed generating runner id: %w", err)
	}

	workload := &AWSFunctionWorkload{
		id:         newUUID.String(),
		executions: execution.NewLocalStorage(),
		onDestroy:  onDestroy,
		fs:         make(map[dto.FilePath][]byte),
	}
	workload.InactivityTimer = NewInactivityTimer(workload, onDestroy)
	return workload, nil
}

func (w *AWSFunctionWorkload) ID() string {
	return w.id
}

func (w *AWSFunctionWorkload) MappedPorts() []*dto.MappedPort {
	panic("implement me")
}

func (w *AWSFunctionWorkload) StoreExecution(_ string, _ *dto.ExecutionRequest) {
	panic("implement me")
}

func (w *AWSFunctionWorkload) ExecutionExists(_ string) bool {
	panic("implement me")
}

func (w *AWSFunctionWorkload) ExecuteInteractively(_ string, _ io.ReadWriter, _, _ io.Writer) (
	exit <-chan ExitInfo, cancel context.CancelFunc, err error) {
	panic("implement me")
}

func (w *AWSFunctionWorkload) UpdateFileSystem(_ *dto.UpdateFileSystemRequest) error {
	panic("implement me")
}

func (w *AWSFunctionWorkload) Destroy() error {
	panic("implement me")
}
