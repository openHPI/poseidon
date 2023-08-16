package util

import (
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

// RetryExponentialAttempts executes the passed function
// with exponentially increasing time in between starting at the passed sleep duration
// up to a maximum of attempts tries.
func RetryExponentialAttempts(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		err = f()
		if err == nil {
			return nil
		} else {
			log.WithField("count", i).WithError(err).Debug("retrying after error")
			time.Sleep(sleep)
			sleep *= 2
		}
	}
	return err
}

func RetryExponentialDuration(sleep time.Duration, f func() error) error {
	return RetryExponentialAttempts(MaxConnectionRetriesExponential, sleep, f)
}

func RetryExponential(f func() error) error {
	return RetryExponentialDuration(InitialWaitingDuration, f)
}
