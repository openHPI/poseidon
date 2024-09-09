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

func (s *MainTestSuite) TestSentryDebugWriter_regression_issue_678() {
	buf := &bytes.Buffer{}
	debugWriter := NewSentryDebugWriter(s.TestCtx, buf)

	innerData := "HOSTNAME=4bf0a8e5dbe4\r\nLANGUAGE=en_US.UTF-8\r\nJAVA_HOME=/usr/lib/jvm/java-8-openjdk-arm64\r\nJUNIT=/usr/java/lib/junit-4.13.2.jar\r\nPWD=/workspace\r\n_=/usr/bin/env\r\nHOME=/workspace\r\nLANG=en_US.UTF-8\r\nCOLUMNS=10000\r\nTERM=ansi\r\nUSER=user\r\nSHLVL=2\r\nANTLR=/usr/java/lib/antlr-4.5.3-complete.jar\r\nCODEOCEAN=true\r\nCLASSPATH=.:/usr/java/lib/hamcrest-3.0.jar:/usr/java/lib/junit-4.13.2.jar:/usr/java/lib/antlr-4.5.3-complete.jar:/usr/java/lib/antlr-java8.jar\r\nHAMCREST=/usr/java/lib/hamcrest-3.0.jar\r\nLC_ALL=en_US.UTF-8\r\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\r\nANTLR_JAVA8=/usr/java/lib/antlr-java8.jar\r\nUID=1001\r\nDEBIAN_FRONTEND=teletype\r\n" //nolint:lll // regression payload
	data := "\x1ePoseidon env 1725462286085\x1e" + innerData + "\x1ePoseidon exit 0 1725462286087\x1e"
	count, err := debugWriter.Write([]byte(data))

	s.Require().NoError(err)
	s.Equal(len(data), count)
	s.Equal(innerData, buf.String())
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
