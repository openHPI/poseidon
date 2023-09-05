package nullio

import (
	"bytes"
	"net/http"
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

func (s *MainTestSuite) TestContentLengthWriter_Write() {
	header := http.Header(make(map[string][]string))
	buf := &responseWriterStub{header: header}
	writer := &ContentLengthWriter{Target: buf}
	part1 := []byte("-rw-rw-r-- 1 kali ka")
	contentLength := "42"
	part2 := []byte("li " + contentLength + " 1660763446 flag\nFL")
	part3 := []byte("AG")

	count, err := writer.Write(part1)
	s.Require().NoError(err)
	s.Equal(len(part1), count)
	s.Empty(buf.String())
	s.Equal("", header.Get("Content-Length"))

	count, err = writer.Write(part2)
	s.Require().NoError(err)
	s.Equal(len(part2), count)
	s.Equal("FL", buf.String())
	s.Equal(contentLength, header.Get("Content-Length"))

	count, err = writer.Write(part3)
	s.Require().NoError(err)
	s.Equal(len(part3), count)
	s.Equal("FLAG", buf.String())
	s.Equal(contentLength, header.Get("Content-Length"))
}
