package nullio

import (
	"context"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
	"io"
	"testing"
	"time"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestReader_Read() {
	read := func(reader io.Reader, ret chan<- bool) {
		p := make([]byte, 0, 5)
		_, err := reader.Read(p)
		s.ErrorIs(io.EOF, err)
		close(ret)
	}

	s.Run("WithContext_DoesNotReturnImmediately", func() {
		readingContext, cancel := context.WithCancel(context.Background())
		defer cancel()

		readerReturned := make(chan bool)
		go read(&Reader{readingContext}, readerReturned)

		select {
		case <-readerReturned:
			s.Fail("The reader returned before the timeout was reached")
		case <-time.After(tests.ShortTimeout):
		}
	})

	s.Run("WithoutContext_DoesReturnImmediately", func() {
		readerReturned := make(chan bool)
		go read(&Reader{}, readerReturned)

		select {
		case <-readerReturned:
		case <-time.After(tests.ShortTimeout):
			s.Fail("The reader returned before the timeout was reached")
		}
	})
}

func (s *MainTestSuite) TestReadWriterWritesEverything() {
	readWriter := &ReadWriter{}
	p := []byte{1, 2, 3}
	n, err := readWriter.Write(p)
	s.NoError(err)
	s.Equal(len(p), n)
}
