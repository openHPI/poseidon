package nullio

import (
	"context"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
	"time"
)

const shortTimeout = 100 * time.Millisecond

func TestReaderDoesNotReturnImmediately(t *testing.T) {
	readingContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := &Reader{readingContext}
	readerReturned := make(chan bool)
	go func() {
		p := make([]byte, 0, 5)
		_, err := reader.Read(p)
		assert.ErrorIs(t, io.EOF, err)
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

func TestReadWriterWritesEverything(t *testing.T) {
	readWriter := &ReadWriter{}
	p := []byte{1, 2, 3}
	n, err := readWriter.Write(p)
	assert.NoError(t, err)
	assert.Equal(t, len(p), n)
}
