package nomad

import (
	"bytes"
)

func (s *MainTestSuite) TestSentryDebugWriter_Write() {
	buf := &bytes.Buffer{}
	debugWriter := NewSentryDebugWriter(s.TestCtx, buf)

	description := "TestDebugMessageDescription"
	data := "\x1EPoseidon " + description + " 1676646791482\x1E"
	count, err := debugWriter.Write([]byte(data))

	s.Require().NoError(err)
	s.Equal(len(data), count)
	s.NotContains(buf.String(), description)
}

func (s *MainTestSuite) TestSentryDebugWriter_WriteComposed() {
	buf := &bytes.Buffer{}
	debugWriter := NewSentryDebugWriter(s.TestCtx, buf)

	data := "Hello World!\r\n\x1EPoseidon unset 1678540012404\x1E\x1EPoseidon /sbin/setuser user 1678540012408\x1E"
	count, err := debugWriter.Write([]byte(data))

	s.Require().NoError(err)
	s.Equal(len(data), count)
	s.Contains(buf.String(), "Hello World!")
}

func (s *MainTestSuite) TestSentryDebugWriter_Close() {
	buf := &bytes.Buffer{}
	w := NewSentryDebugWriter(s.TestCtx, buf)
	s.Require().Empty(w.lastSpan.Tags)

	w.Close(42)
	s.Require().Contains(w.lastSpan.Tags, "exit_code")
	s.Equal("42", w.lastSpan.Tags["exit_code"])
}

func (s *MainTestSuite) TestSentryDebugWriter_handleTimeDebugMessage() {
	buf := &bytes.Buffer{}
	debugWriter := NewSentryDebugWriter(s.TestCtx, buf)
	s.Require().Equal("nomad.execute.connect", debugWriter.lastSpan.Op)

	description := "TestDebugMessageDescription"
	match := map[string][]byte{"time": []byte("1676646791482"), "text": []byte(description)}
	debugWriter.handleTimeDebugMessage(match)
	s.Equal("nomad.execute.bash", debugWriter.lastSpan.Op)
	s.Equal(description, debugWriter.lastSpan.Description)
}
