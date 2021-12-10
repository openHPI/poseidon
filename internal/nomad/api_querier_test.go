package nomad

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWebsocketErrorNeedsToBeUnwrapped(t *testing.T) {
	rawError := &websocket.CloseError{Code: websocket.CloseNormalClosure}
	err := fmt.Errorf("websocket closed before receiving exit code: %w", rawError)

	assert.False(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
	rootCause := errors.Unwrap(err)
	assert.True(t, websocket.IsCloseError(rootCause, websocket.CloseNormalClosure))
}
