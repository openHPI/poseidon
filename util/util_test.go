package util

import (
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"testing"
)

func TestNullReaderDoesNotReturnImmediately(t *testing.T) {
	reader := &NullReader{}
	readerReturned := make(chan bool)
	go func() {
		p := make([]byte, 5)
		_, _ = reader.Read(p)
		close(readerReturned)
	}()
	assert.False(t, tests.ChannelReceivesSomething(readerReturned, tests.ShortTimeout))
}
