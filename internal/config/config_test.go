package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openHPI/poseidon/tests"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	getServerPort     = func(c *configuration) interface{} { return c.Server.Port }
	getNomadToken     = func(c *configuration) interface{} { return c.Nomad.Token }
	getNomadTLSActive = func(c *configuration) interface{} { return c.Nomad.TLS.Active }
	getAWSFunctions   = func(c *configuration) interface{} { return c.AWS.Functions }
)

func newTestConfiguration() *configuration {
	return &configuration{
		Server: server{
			Address: "127.0.0.1",
			Port:    3000,
		},
		Nomad: Nomad{
			Address: "127.0.0.2",
			Port:    4646,
			Token:   "SECRET",
			TLS: TLS{
				Active: false,
			},
		},
		Logger: Logger{
			Level: "INFO",
		},
	}
}

func (c *configuration) getReflectValue() reflect.Value {
	return reflect.ValueOf(c).Elem()
}

// writeConfigurationFile creates a file on disk and returns the path to it.
func writeConfigurationFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	directory := t.TempDir()
	filePath := filepath.Join(directory, name)
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal("Could not create config file")
	}
	defer file.Close()
	_, err = file.Write(content)
	require.NoError(t, err)
	return filePath
}

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestCallingInitConfigTwiceReturnsError() {
	configurationInitialized = false
	err := InitConfig()
	s.Require().NoError(err)
	err = InitConfig()
	s.Error(err)
}

func (s *MainTestSuite) TestCallingInitConfigTwiceDoesNotChangeConfig() {
	configurationInitialized = false
	err := InitConfig()
	s.Require().NoError(err)
	Config = newTestConfiguration()
	filePath := writeConfigurationFile(s.T(), "test.yaml", []byte("server:\n  port: 5000\n"))
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = append(os.Args, "-config", filePath)
	err = InitConfig()
	s.Require().Error(err)
	s.Equal(3000, Config.Server.Port)
}

func (s *MainTestSuite) TestReadEnvironmentVariables() {
	var environmentTests = []struct {
		variableSuffix string
		valueToSet     string
		expectedValue  interface{}
		getTargetField func(*configuration) interface{}
	}{
		{"SERVER_PORT", "4000", 4000, getServerPort},
		{"SERVER_PORT", "hello", 3000, getServerPort},
		{"NOMAD_TOKEN", "ACCESS", "ACCESS", getNomadToken},
		{"NOMAD_TLS_ACTIVE", "true", true, getNomadTLSActive},
		{"NOMAD_TLS_ACTIVE", "hello", false, getNomadTLSActive},
		{"AWS_FUNCTIONS", "java11Exec go118Exec", []string{"java11Exec", "go118Exec"}, getAWSFunctions},
	}
	prefix := "POSEIDON_TEST"
	for _, testCase := range environmentTests {
		config := newTestConfiguration()
		environmentVariable := fmt.Sprintf("%s_%s", prefix, testCase.variableSuffix)
		_ = os.Setenv(environmentVariable, testCase.valueToSet)
		readFromEnvironment(prefix, config.getReflectValue())
		_ = os.Unsetenv(environmentVariable)
		s.Equal(testCase.expectedValue, testCase.getTargetField(config))
	}
}

func (s *MainTestSuite) TestReadEnvironmentIgnoresNonPointerValue() {
	config := newTestConfiguration()
	_ = os.Setenv("POSEIDON_TEST_SERVER_PORT", "4000")
	readFromEnvironment("POSEIDON_TEST", reflect.ValueOf(config))
	_ = os.Unsetenv("POSEIDON_TEST_SERVER_PORT")
	s.Equal(3000, config.Server.Port)
}

func (s *MainTestSuite) TestReadEnvironmentIgnoresNotSupportedType() {
	config := &struct{ Timeout float64 }{1.0}
	_ = os.Setenv("POSEIDON_TEST_TIMEOUT", "2.5")
	readFromEnvironment("POSEIDON_TEST", reflect.ValueOf(config).Elem())
	_ = os.Unsetenv("POSEIDON_TEST_TIMEOUT")
	s.Equal(1, int(config.Timeout))
}

func (s *MainTestSuite) TestUnsetEnvironmentVariableDoesNotChangeConfig() {
	config := newTestConfiguration()
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	s.Equal("INFO", config.Logger.Level)
}

func (s *MainTestSuite) TestReadYamlConfigFile() {
	var yamlTests = []struct {
		content        []byte
		expectedValue  interface{}
		getTargetField func(*configuration) interface{}
	}{
		{[]byte("server:\n  port: 5000\n"), 5000, getServerPort},
		{[]byte("nomad:\n  token: ACCESS\n"), "ACCESS", getNomadToken},
		{[]byte("nomad:\n  tls:\n    active: true\n"), true, getNomadTLSActive},
		{[]byte(""), false, getNomadTLSActive},
		{[]byte("nomad:\n  token:\n"), "SECRET", getNomadToken},
		{[]byte("aws:\n  functions:\n    - java11Exec\n    - go118Exec\n"),
			[]string{"java11Exec", "go118Exec"}, getAWSFunctions},
	}
	for _, testCase := range yamlTests {
		config := newTestConfiguration()
		config.mergeYaml(testCase.content)
		s.Equal(testCase.expectedValue, testCase.getTargetField(config))
	}
}

func (s *MainTestSuite) TestInvalidYamlExitsProgram() {
	logger, hook := test.NewNullLogger()
	// this function is used when calling log.Fatal() and
	// prevents the program from exiting during this test
	logger.ExitFunc = func(code int) {}
	log = logger.WithField("package", "config_test")
	config := newTestConfiguration()
	config.mergeYaml([]byte("logger: level: DEBUG"))
	s.Len(hook.Entries, 1)
	s.Equal(logrus.FatalLevel, hook.LastEntry().Level)
}

func (s *MainTestSuite) TestReadConfigFileOverwritesConfig() {
	Config = newTestConfiguration()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	filePath := writeConfigurationFile(s.T(), "test.yaml", []byte("server:\n  port: 5000\n"))
	os.Args = append(os.Args, "-config", filePath)
	configurationInitialized = false
	err := InitConfig()
	s.Require().NoError(err)
	s.Equal(5000, Config.Server.Port)
}

func (s *MainTestSuite) TestReadNonExistingConfigFileDoesNotOverwriteConfig() {
	Config = newTestConfiguration()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = append(os.Args, "-config", "file_does_not_exist.yaml")
	configurationInitialized = false
	err := InitConfig()
	s.Require().NoError(err)
	s.Equal(3000, Config.Server.Port)
}

func (s *MainTestSuite) TestURLParsing() {
	var urlTests = []struct {
		address        string
		port           int
		tls            bool
		expectedScheme string
		expectedHost   string
	}{
		{"localhost", 3000, false, "http", "localhost:3000"},
		{"127.0.0.1", 4000, true, "https", "127.0.0.1:4000"},
	}
	for _, testCase := range urlTests {
		url := parseURL(testCase.address, testCase.port, testCase.tls)
		s.Equal(testCase.expectedScheme, url.Scheme)
		s.Equal(testCase.expectedHost, url.Host)
	}
}

func (s *MainTestSuite) TestNomadAPIURL() {
	config := newTestConfiguration()
	s.Equal("http", config.Nomad.URL().Scheme)
	s.Equal("127.0.0.2:4646", config.Nomad.URL().Host)
}

func (s *MainTestSuite) TestPoseidonAPIURL() {
	config := newTestConfiguration()
	s.Equal("http", config.Server.URL().Scheme)
	s.Equal("127.0.0.1:3000", config.Server.URL().Host)
}
