package nomad

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
	"testing"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestWebsocketErrorNeedsToBeUnwrapped() {
	rawError := &websocket.CloseError{Code: websocket.CloseNormalClosure}
	err := fmt.Errorf("websocket closed before receiving exit code: %w", rawError)

	s.False(websocket.IsCloseError(err, websocket.CloseNormalClosure))
	rootCause := errors.Unwrap(err)
	s.True(websocket.IsCloseError(rootCause, websocket.CloseNormalClosure))
}
