package util

import (
	"context"
	"fmt"
	"github.com/openHPI/poseidon/pkg/logging"
	"time"
)

var (
	log = logging.GetLogger("util")
	// MaxConnectionRetriesExponential is the default number of retries. It's exported for testing reasons.
	MaxConnectionRetriesExponential = 18
	// InitialWaitingDuration is the default initial duration of waiting after a failed time.
	InitialWaitingDuration = time.Second
)

// RetryExponentialAttemptsContext executes the passed function
// with exponentially increasing time in between starting at the passed sleep duration
// up to a maximum of attempts tries as long as the context is not done.
func RetryExponentialAttemptsContext(
	ctx context.Context, attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return fmt.Errorf("stopped retry due to: %w", ctx.Err())
		} else if err = f(); err == nil {
			return nil
		} else {
			log.WithField("count", i).WithError(err).Debug("retrying after error")
			time.Sleep(sleep)
			sleep *= 2
		}
	}
	return err
}

func RetryExponentialContext(ctx context.Context, f func() error) error {
	return RetryExponentialAttemptsContext(ctx, MaxConnectionRetriesExponential, InitialWaitingDuration, f)
}

func RetryExponentialDuration(sleep time.Duration, f func() error) error {
	return RetryExponentialAttemptsContext(context.Background(), MaxConnectionRetriesExponential, sleep, f)
}

func RetryExponential(f func() error) error {
	return RetryExponentialDuration(InitialWaitingDuration, f)
}
