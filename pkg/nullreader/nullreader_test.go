package nullreader

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"testing"
)

func TestNullReaderDoesNotReturnImmediately(t *testing.T) {
	reader := &NullReader{}
	readerReturned := make(chan bool)
	go func() {
		p := make([]byte, 0, 5)
		_, err := reader.Read(p)
		require.NoError(t, err)
		close(readerReturned)
	}()
	assert.False(t, tests.ChannelReceivesSomething(readerReturned, tests.ShortTimeout))
}
