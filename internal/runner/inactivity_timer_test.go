package runner

import (
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

func TestInactivityTimerTestSuite(t *testing.T) {
	suite.Run(t, new(InactivityTimerTestSuite))
}

type InactivityTimerTestSuite struct {
	suite.Suite
	runner   Runner
	returned chan bool
}

func (s *InactivityTimerTestSuite) SetupTest() {
	s.returned = make(chan bool, 1)
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", tests.DefaultRunnerID).Return(nil)
	s.runner = NewNomadJob(tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error {
		s.returned <- true
		return nil
	})

	s.runner.SetupTimeout(tests.ShortTimeout)
}

func (s *InactivityTimerTestSuite) TearDownTest() {
	s.runner.StopTimeout()
}

func (s *InactivityTimerTestSuite) TestRunnerIsReturnedAfterTimeout() {
	s.True(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestRunnerIsNotReturnedBeforeTimeout() {
	s.False(tests.ChannelReceivesSomething(s.returned, tests.ShortTimeout/2))
}

func (s *InactivityTimerTestSuite) TestResetTimeoutExtendsTheDeadline() {
	time.Sleep(3 * tests.ShortTimeout / 4)
	s.runner.ResetTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 3*tests.ShortTimeout/4),
		"Because of the reset, the timeout should not be reached by now.")
	s.True(tests.ChannelReceivesSomething(s.returned, 5*tests.ShortTimeout/4),
		"After reset, the timout should be reached by now.")
}

func (s *InactivityTimerTestSuite) TestStopTimeoutStopsTimeout() {
	s.runner.StopTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestTimeoutPassedReturnsFalseBeforeDeadline() {
	s.False(s.runner.TimeoutPassed())
}

func (s *InactivityTimerTestSuite) TestTimeoutPassedReturnsTrueAfterDeadline() {
	time.Sleep(2 * tests.ShortTimeout)
	s.True(s.runner.TimeoutPassed())
}

func (s *InactivityTimerTestSuite) TestTimerIsNotResetAfterDeadline() {
	time.Sleep(2 * tests.ShortTimeout)
	// We need to empty the returned channel so Return can send to it again.
	tests.ChannelReceivesSomething(s.returned, 0)
	s.runner.ResetTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestSetupTimeoutStopsOldTimeout() {
	s.runner.SetupTimeout(3 * tests.ShortTimeout)
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
	s.True(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestTimerIsInactiveWhenDurationIsZero() {
	s.runner.SetupTimeout(0)
	s.False(tests.ChannelReceivesSomething(s.returned, tests.ShortTimeout))
}
