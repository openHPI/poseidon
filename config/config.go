package config

import (
	"flag"
	"fmt"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gopkg.in/yaml.v3"
	"os"
	"reflect"
	"strconv"
	"strings"
)

var (
	Config = &configuration{
		Server: server{
			Address: "127.0.0.1",
			Port:    3000,
		},
		Nomad: nomad{
			Address: "",
			Token:   "",
			TLS:     false,
		},
		Logger: logger{
			Level: "INFO",
		},
	}
	log = logging.GetLogger("config")
)

type server struct {
	Address string
	Port    int
}

type nomad struct {
	Address string
	Token   string
	TLS     bool
}

type logger struct {
	Level string
}

type configuration struct {
	Server server
	Nomad  nomad
	Logger logger
}

func InitConfig() {
	content := readConfigFile()
	Config.mergeYaml(content)
	Config.mergeEnvironmentVariables()
}

func readConfigFile() []byte {
	var configFilePath string
	flag.StringVar(&configFilePath, "config", "./configuration.yaml", "path of the yaml config file")
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		log.WithError(err).Info("Using default configuration...")
	}
	return data
}

func (c *configuration) mergeYaml(content []byte) {
	if err := yaml.Unmarshal(content, c); err != nil {
		log.WithError(err).Fatal("Could not parse configuration file")
	}
}

func (c *configuration) mergeEnvironmentVariables() {
	readFromEnvironment("POSEIDON", reflect.ValueOf(c).Elem())
}

func readFromEnvironment(prefix string, value reflect.Value) {
	if !value.CanSet() || !value.CanInterface() {
		return
	}

	if value.Kind() != reflect.Struct {
		content, ok := os.LookupEnv(prefix)

		logEntry := log.
			WithField("prefix", prefix).
			WithField("content", content)

		if !ok {
			return
		}
		switch value.Kind() {
		case reflect.String:
			value.SetString(content)
		case reflect.Int:
			integer, err := strconv.Atoi(content)
			if err != nil {
				logEntry.Warn("Could not parse environment variable as integer")
				return
			}
			value.SetInt(int64(integer))
		case reflect.Bool:
			boolean, err := strconv.ParseBool(content)
			if err != nil {
				logEntry.Warn("Could not parse environment variable as boolean")
				return
			}
			value.SetBool(boolean)
		default:
			// ignore this field
			logEntry.WithField("type", value.Type().Name()).Warn("Setting configuration option via environment variables is not supported")
		}
	} else {
		for i := 0; i < value.NumField(); i++ {
			fieldName := value.Type().Field(i).Name
			newPrefix := fmt.Sprintf("%s_%s", prefix, strings.ToUpper(fieldName))
			readFromEnvironment(newPrefix, value.Field(i))
		}
	}
}
