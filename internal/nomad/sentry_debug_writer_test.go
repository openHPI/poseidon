package nomad

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSentryDebugWriter_Write(t *testing.T) {
	buf := &bytes.Buffer{}
	w := SentryDebugWriter{Target: buf, Ctx: context.Background()}

	description := "TestDebugMessageDescription"
	data := "\x1EPoseidon " + description + " 1676646791482\x1E"
	count, err := w.Write([]byte(data))

	require.NoError(t, err)
	assert.Equal(t, len(data), count)
	assert.NotContains(t, buf.String(), description)
}

func TestSentryDebugWriter_Close(t *testing.T) {
	buf := &bytes.Buffer{}
	s := NewSentryDebugWriter(buf, context.Background())
	require.Empty(t, s.lastSpan.Tags)

	s.Close(42)
	require.Contains(t, s.lastSpan.Tags, "exit_code")
	assert.Equal(t, "42", s.lastSpan.Tags["exit_code"])
}

func TestSentryDebugWriter_handleTimeDebugMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	s := NewSentryDebugWriter(buf, context.Background())
	require.Equal(t, "nomad.execute.connect", s.lastSpan.Op)

	description := "TestDebugMessageDescription"
	match := map[string][]byte{"time": []byte("1676646791482"), "text": []byte(description)}
	s.handleTimeDebugMessage(match)
	assert.Equal(t, "nomad.execute.bash", s.lastSpan.Op)
	assert.Equal(t, description, s.lastSpan.Description)
}
