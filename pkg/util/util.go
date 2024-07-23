package util

import (
	"context"
	"errors"
	"time"

	"github.com/openHPI/poseidon/pkg/logging"
)

var (
	log = logging.GetLogger("util")
	// MaxConnectionRetriesExponential is the default number of retries. It's exported for testing reasons.
	MaxConnectionRetriesExponential = 18
	// InitialWaitingDuration is the default initial duration of waiting after a failed time.
	InitialWaitingDuration = time.Second
	ErrRetryContextDone    = errors.New("the retry context is done")
)

func retryExponential(ctx context.Context, sleep time.Duration, f func() error) func() error {
	return func() error {
		err := f()
		if err != nil {
			select {
			case <-ctx.Done():
				err = ErrRetryContextDone
			case <-time.After(sleep):
				sleep *= 2
			}
		}

		return err
	}
}

func retryConstant(ctx context.Context, sleep time.Duration, f func() error) func() error {
	return func() error {
		err := f()
		if err != nil {
			select {
			case <-ctx.Done():
				return ErrRetryContextDone
			case <-time.After(sleep):
			}
		}

		return err
	}
}

func retryAttempts(maxAttempts int, f func() error) (err error) {
	for i := range maxAttempts {
		err = f()
		if err == nil {
			return nil
		} else if errors.Is(err, ErrRetryContextDone) {
			return err
		}
		log.WithField("count", i).WithError(err).Debug("retrying after error")
	}
	return err
}

// RetryExponentialWithContext executes the passed function with exponentially increasing time starting with one second
// up to a default maximum number of attempts as long as the context is not done.
func RetryExponentialWithContext(ctx context.Context, f func() error) error {
	return retryAttempts(MaxConnectionRetriesExponential, retryExponential(ctx, InitialWaitingDuration, f))
}

// RetryExponential executes the passed function with exponentially increasing time starting with one second
// up to a default maximum number of attempts.
func RetryExponential(f func() error) error {
	return retryAttempts(MaxConnectionRetriesExponential,
		retryExponential(context.Background(), InitialWaitingDuration, f))
}

// RetryConstantAttemptsWithContext executes the passed function with a constant retry delay of one second
// up to the passed maximum number of attempts as long as the context is not done.
func RetryConstantAttemptsWithContext(ctx context.Context, attempts int, f func() error) error {
	return retryAttempts(attempts, retryConstant(ctx, InitialWaitingDuration, f))
}
