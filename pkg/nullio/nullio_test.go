package nullio

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const shortTimeout = 100 * time.Millisecond

func TestReaderDoesNotReturnImmediately(t *testing.T) {
	reader := &Reader{}
	readerReturned := make(chan bool)
	go func() {
		p := make([]byte, 0, 5)
		_, err := reader.Read(p)
		require.NoError(t, err)
		close(readerReturned)
	}()

	var received bool
	select {
	case <-readerReturned:
		received = true
	case <-time.After(shortTimeout):
		received = false
	}

	assert.False(t, received)
}
