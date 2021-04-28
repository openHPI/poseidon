package config

import (
	"github.com/stretchr/testify/assert"
	"os"
	"reflect"
	"testing"
)

func newTestConfiguration() *configuration {
	return &configuration{
		Server: server{
			Address: "127.0.0.1",
			Port:    3000,
		},
		Nomad: nomad{
			Address: "127.0.0.2",
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

func TestIntEnvironmentVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	_ = os.Setenv("POSEIDON_TEST_SERVER_PORT", "4000")
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	_ = os.Unsetenv("POSEIDON_TEST_SERVER_PORT")
	assert.Equal(t, 4000, config.Server.Port)
}

func TestStringEnvironmentVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	_ = os.Setenv("POSEIDON_TEST_NOMAD_TOKEN", "ACCESS")
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	_ = os.Unsetenv("POSEIDON_TEST_NOMAD_TOKEN")
	assert.Equal(t, "ACCESS", config.Nomad.Token)
}

func TestBoolEnvironmentVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	_ = os.Setenv("POSEIDON_TEST_NOMAD_TLS", "true")
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	_ = os.Unsetenv("POSEIDON_TEST_NOMAD_TLS")
	assert.Equal(t, true, config.Nomad.TLS)
}

func TestUnsetEnvironmentVariableDoesNotChangeConfig(t *testing.T) {
	config := newTestConfiguration()
	readFromEnvironment("POSEIDON_TEST", config.getReflectValue())
	assert.Equal(t, "INFO", config.Logger.Level)
}

func TestIntYamlVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	config.mergeYaml([]byte("server:\n  port: 5000\n"))
	assert.Equal(t, 5000, config.Server.Port)
}

func TestStringYamlVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	config.mergeYaml([]byte("logger:\n  level: DEBUG\n"))
	assert.Equal(t, "DEBUG", config.Logger.Level)
}

func TestBoolYamlVariableOverwritesConfig(t *testing.T) {
	config := newTestConfiguration()
	config.mergeYaml([]byte("nomad:\n  tls: true\n"))
	assert.Equal(t, true, config.Nomad.TLS)
}

func TestMissingYamlVariableDoesNotChangeConfig(t *testing.T) {
	config := newTestConfiguration()
	config.mergeYaml([]byte(""))
	assert.Equal(t, "127.0.0.2", config.Nomad.Address)
}

func TestUnsetYamlVariableDoesNotChangeConfig(t *testing.T) {
	config := newTestConfiguration()
	config.mergeYaml([]byte("nomad:\n  address:\n"))
	assert.Equal(t, "127.0.0.2", config.Nomad.Address)
}
