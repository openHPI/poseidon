package runner

import (
	"errors"
	"sync"
	"time"

	"github.com/openHPI/poseidon/pkg/dto"
)

// InactivityTimer is a wrapper around a timer that is used to delete a a Runner after some time of inactivity.
type InactivityTimer interface {
	// SetupTimeout starts the timeout after a runner gets deleted.
	SetupTimeout(duration time.Duration)

	// ResetTimeout resets the current timeout so that the runner gets deleted after the time set in Setup from now.
	// It does not make an already expired timer run again.
	ResetTimeout()

	// StopTimeout stops the timeout but does not remove the runner.
	StopTimeout()

	// TimeoutPassed returns true if the timeout expired and false otherwise.
	TimeoutPassed() bool
}

type TimerState uint8

const (
	TimerInactive TimerState = 0
	TimerRunning  TimerState = 1
	TimerExpired  TimerState = 2
)

var (
	ErrorRunnerInactivityTimeout DestroyReason = errors.New("runner inactivity timeout exceeded")
	ErrorExecutionTimeout                      = errors.New("execution timeout exceeded")
)

type InactivityTimerImplementation struct {
	timer     *time.Timer
	duration  time.Duration
	state     TimerState
	runner    Runner
	onDestroy DestroyRunnerHandler
	mu        sync.Mutex
}

func NewInactivityTimer(runner Runner, onDestroy DestroyRunnerHandler) InactivityTimer {
	return &InactivityTimerImplementation{
		state:     TimerInactive,
		runner:    runner,
		onDestroy: onDestroy,
	}
}

func (t *InactivityTimerImplementation) SetupTimeout(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Stop old timer if present.
	if t.timer != nil {
		t.timer.Stop()
	}
	if duration == 0 {
		t.state = TimerInactive
		return
	}
	t.state = TimerRunning
	t.duration = duration

	t.timer = time.AfterFunc(duration, func() {
		t.mu.Lock()
		t.state = TimerExpired
		// The timer must be unlocked here already in order to avoid a deadlock with the call to StopTimout in Manager.Return.
		t.mu.Unlock()
		err := t.onDestroy(t.runner)
		if err != nil {
			log.WithError(err).WithField(dto.KeyRunnerID, t.runner.ID()).
				Warn("Returning runner after inactivity caused an error")
		} else {
			log.WithField(dto.KeyRunnerID, t.runner.ID()).Info("Returning runner due to inactivity timeout")
		}
	})
}

func (t *InactivityTimerImplementation) ResetTimeout() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TimerRunning {
		// The timer has already expired or been stopped. We don't want to restart it.
		return
	}
	if t.timer.Stop() {
		t.timer.Reset(t.duration)
	} else {
		log.Error("Timer is in state running but stopped. This should never happen")
	}
}

func (t *InactivityTimerImplementation) StopTimeout() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TimerRunning {
		return
	}
	t.timer.Stop()
	t.state = TimerInactive
}

func (t *InactivityTimerImplementation) TimeoutPassed() bool {
	return t.state == TimerExpired
}
