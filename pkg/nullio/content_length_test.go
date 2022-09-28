package nullio

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type responseWriterStub struct {
	bytes.Buffer
	header http.Header
}

func (r *responseWriterStub) Header() http.Header {
	return r.header
}
func (r *responseWriterStub) WriteHeader(_ int) {
}

func TestContentLengthWriter_Write(t *testing.T) {
	header := http.Header(make(map[string][]string))
	buf := &responseWriterStub{header: header}
	writer := &ContentLengthWriter{Target: buf}
	part1 := []byte("-rw-rw-r-- 1 kali ka")
	contentLength := "42"
	part2 := []byte("li " + contentLength + " 1660763446 flag\nFL")
	part3 := []byte("AG")

	count, err := writer.Write(part1)
	require.NoError(t, err)
	assert.Equal(t, len(part1), count)
	assert.Empty(t, buf.String())
	assert.Equal(t, "", header.Get("Content-Length"))

	count, err = writer.Write(part2)
	require.NoError(t, err)
	assert.Equal(t, len(part2), count)
	assert.Equal(t, "FL", buf.String())
	assert.Equal(t, contentLength, header.Get("Content-Length"))

	count, err = writer.Write(part3)
	require.NoError(t, err)
	assert.Equal(t, len(part3), count)
	assert.Equal(t, "FLAG", buf.String())
	assert.Equal(t, contentLength, header.Get("Content-Length"))
}
