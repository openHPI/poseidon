package nomad

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSentryDebugWriter_Write(t *testing.T) {
	buf := &bytes.Buffer{}
	w := SentryDebugWriter{Target: buf}

	description := "TestDebugMessageDescription"
	data := "\x1EPoseidon " + description + " 1676646791482\x1E"
	count, err := w.Write([]byte(data))

	require.NoError(t, err)
	assert.Equal(t, len(data), count)
	assert.NotContains(t, buf.String(), description)
}
