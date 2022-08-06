package nullio

import (
	"context"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
	"time"
)

const shortTimeout = 100 * time.Millisecond

func TestReader_Read(t *testing.T) {
	read := func(reader io.Reader, ret chan<- bool) {
		p := make([]byte, 0, 5)
		_, err := reader.Read(p)
		assert.ErrorIs(t, io.EOF, err)
		close(ret)
	}

	t.Run("WithContext_DoesNotReturnImmediately", func(t *testing.T) {
		readingContext, cancel := context.WithCancel(context.Background())
		defer cancel()

		readerReturned := make(chan bool)
		go read(&Reader{readingContext}, readerReturned)

		select {
		case <-readerReturned:
			assert.Fail(t, "The reader returned before the timeout was reached")
		case <-time.After(shortTimeout):
		}
	})

	t.Run("WithoutContext_DoesReturnImmediately", func(t *testing.T) {
		readerReturned := make(chan bool)
		go read(&Reader{}, readerReturned)

		select {
		case <-readerReturned:
		case <-time.After(shortTimeout):
			assert.Fail(t, "The reader returned before the timeout was reached")
		}
	})
}

func TestReadWriterWritesEverything(t *testing.T) {
	readWriter := &ReadWriter{}
	p := []byte{1, 2, 3}
	n, err := readWriter.Write(p)
	assert.NoError(t, err)
	assert.Equal(t, len(p), n)
}
