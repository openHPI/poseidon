package nomad

import (
	"bytes"
)

func (s *MainTestSuite) TestSentryDebugWriter_Write() {
	buf := &bytes.Buffer{}
	w := SentryDebugWriter{Target: buf, Ctx: s.TestCtx}

	description := "TestDebugMessageDescription"
	data := "\x1EPoseidon " + description + " 1676646791482\x1E"
	count, err := w.Write([]byte(data))

	s.Require().NoError(err)
	s.Equal(len(data), count)
	s.NotContains(buf.String(), description)
}

func (s *MainTestSuite) TestSentryDebugWriter_WriteComposed() {
	buf := &bytes.Buffer{}
	w := SentryDebugWriter{Target: buf, Ctx: s.TestCtx}

	data := "Hello World!\r\n\x1EPoseidon unset 1678540012404\x1E\x1EPoseidon /sbin/setuser user 1678540012408\x1E"
	count, err := w.Write([]byte(data))

	s.Require().NoError(err)
	s.Equal(len(data), count)
	s.Contains(buf.String(), "Hello World!")
}

func (s *MainTestSuite) TestSentryDebugWriter_Close() {
	buf := &bytes.Buffer{}
	w := NewSentryDebugWriter(buf, s.TestCtx)
	s.Require().Empty(w.lastSpan.Tags)

	w.Close(42)
	s.Require().Contains(w.lastSpan.Tags, "exit_code")
	s.Equal("42", w.lastSpan.Tags["exit_code"])
}

func (s *MainTestSuite) TestSentryDebugWriter_handleTimeDebugMessage() {
	buf := &bytes.Buffer{}
	w := NewSentryDebugWriter(buf, s.TestCtx)
	s.Require().Equal("nomad.execute.connect", w.lastSpan.Op)

	description := "TestDebugMessageDescription"
	match := map[string][]byte{"time": []byte("1676646791482"), "text": []byte(description)}
	w.handleTimeDebugMessage(match)
	s.Equal("nomad.execute.bash", w.lastSpan.Op)
	s.Equal(description, w.lastSpan.Description)
}
