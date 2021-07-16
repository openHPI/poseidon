package config

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	getServerPort = func(c *configuration) interface{} { return c.Server.Port }
	getNomadToken = func(c *configuration) interface{} { return c.Nomad.Token }
	getNomadTLS   = func(c *configuration) interface{} { return c.Nomad.TLS }
)

func newTestConfiguration() *configuration {
	return &configuration{
		Server: server{
			Address: "127.0.0.1",
			Port:    3000,
		},
		Nomad: nomad{
			Address: "127.0.0.2",
			Port:    4646,
			Token:   "SECRET",
			TLS:     false,
		},
		Logger: logger{
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

func TestCallingInitConfigTwiceReturnsError(t *testing.T) {
	configurationInitialized = false
	err := InitConfig()
	assert.NoError(t, err)
	err = InitConfig()
	assert.Error(t, err)
}

func TestCallingInitConfigTwiceDoesNotChangeConfig(t *testing.T) {
	configurationInitialized = false
	err := InitConfig()
	require.NoError(t, err)
	Config = newTestConfiguration()
	filePath := writeConfigurationFile(t, "test.yaml", []byte("server:\n  port: 5000\n"))
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = append(os.Args, "-config", filePath)
	err = InitConfig()
	require.Error(t, err)
	assert.Equal(t, 3000, Config.Server.Port)
}

func TestReadEnvironmentVariables(t *testing.T) {
	var environmentTests = []struct {
		variableSuffix string
		valueToSet     string
		expectedValue  interface{}
		getTargetField func(*configuration) interface{}
	}{
		{"SERVER_PORT", "4000", 4000, getServerPort},
		{"SERVER_PORT", "hello", 3000, getServerPort},
		{"NOMAD_TOKEN", "ACCESS", "ACCESS", getNomadToken},
		{"NOMAD_TLS", "true", true, getNomadTLS},
		{"NOMAD_TLS", "hello", false, getNomadTLS},
	}
	prefix := "POSEIDON_TEST"
	for _, testCase := range environmentTests {
		config := newTestConfiguration()
		environmentVariable := fmt.Sprintf("%s_%s", prefix, testCase.variableSuffix)
		_ = os.Setenv(environmentVariable, testCase.valueToSet)
		readFromEnvironment(prefix, config.getReflectValue())
		_ = os.Unsetenv(environmentVariable)
		assert.Equal(t, testCase.expectedValue, testCase.getTargetField(config))
	}
}

func TestReadEnvironmentIgnoresNonPointerValue(t *testing.T) {
	config := newTestConfiguration()
	_ = os.Setenv("POSEIDON_TEST_SERVER_PORT", "4000")
	readFromEnvironment("POSEIDON_TEST", reflect.ValueOf(config))
	_ = os.Unsetenv("POSEIDON_TEST_SERVER_PORT")
	assert.Equal(t, 3000, config.Server.Port)
}

func TestReadEnvironmentIgnoresNotSupportedType(t *testing.T) {
	config := &struct{ Timeout float64 }{1.0}
	_ = os.Setenv("POSEIDON_TEST_TIMEOUT", "2.5")
	readFromEnvironment("POSEIDON_TEST", reflect.ValueOf(config).Elem())
	_ = os.Unsetenv("POSEIDON_TEST_TIMEOUT")
	assert.Equal(t, 1.0, config.Timeout)
}

func TestUnsetEnvironmentVariableDoesNotChangeConfig(t *testing.T) {
	config := newTestConfiguration()
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	assert.Equal(t, "INFO", config.Logger.Level)
}

func TestReadYamlConfigFile(t *testing.T) {
	var yamlTests = []struct {
		content        []byte
		expectedValue  interface{}
		getTargetField func(*configuration) interface{}
	}{
		{[]byte("server:\n  port: 5000\n"), 5000, getServerPort},
		{[]byte("nomad:\n  token: ACCESS\n"), "ACCESS", getNomadToken},
		{[]byte("nomad:\n  tls: true\n"), true, getNomadTLS},
		{[]byte(""), false, getNomadTLS},
		{[]byte("nomad:\n  token:\n"), "SECRET", getNomadToken},
	}
	for _, testCase := range yamlTests {
		config := newTestConfiguration()
		config.mergeYaml(testCase.content)
		assert.Equal(t, testCase.expectedValue, testCase.getTargetField(config))
	}
}

func TestInvalidYamlExitsProgram(t *testing.T) {
	logger, hook := test.NewNullLogger()
	// this function is used when calling log.Fatal() and
	// prevents the program from exiting during this test
	logger.ExitFunc = func(code int) {}
	log = logger.WithField("package", "config_test")
	config := newTestConfiguration()
	config.mergeYaml([]byte("logger: level: DEBUG"))
	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.FatalLevel, hook.LastEntry().Level)
}

func TestReadConfigFileOverwritesConfig(t *testing.T) {
	Config = newTestConfiguration()
	filePath := writeConfigurationFile(t, "test.yaml", []byte("server:\n  port: 5000\n"))
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = append(os.Args, "-config", filePath)
	configurationInitialized = false
	err := InitConfig()
	require.NoError(t, err)
	assert.Equal(t, 5000, Config.Server.Port)
}

func TestReadNonExistingConfigFileDoesNotOverwriteConfig(t *testing.T) {
	Config = newTestConfiguration()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = append(os.Args, "-config", "file_does_not_exist.yaml")
	configurationInitialized = false
	err := InitConfig()
	require.NoError(t, err)
	assert.Equal(t, 3000, Config.Server.Port)
}

func TestURLParsing(t *testing.T) {
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
		assert.Equal(t, testCase.expectedScheme, url.Scheme)
		assert.Equal(t, testCase.expectedHost, url.Host)
	}
}

func TestNomadAPIURL(t *testing.T) {
	config := newTestConfiguration()
	assert.Equal(t, "http", config.NomadAPIURL().Scheme)
	assert.Equal(t, "127.0.0.2:4646", config.NomadAPIURL().Host)
}

func TestPoseidonAPIURL(t *testing.T) {
	config := newTestConfiguration()
	assert.Equal(t, "http", config.PoseidonAPIURL().Scheme)
	assert.Equal(t, "127.0.0.1:3000", config.PoseidonAPIURL().Host)
}
