package config

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
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
		log.Printf("Using default configuration... (%v)", err)
	}
	return data
}

func (c *configuration) mergeYaml(content []byte) {
	if err := yaml.Unmarshal(content, c); err != nil {
		log.Fatalf("Could not parse configuration: %v", err)
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
		if !ok {
			return
		}
		switch value.Kind() {
		case reflect.String:
			value.SetString(content)
		case reflect.Int:
			integer, err := strconv.Atoi(content)
			if err != nil {
				log.Printf("Could not parse environment variable %s with value '%q' as integer", prefix, content)
				return
			}
			value.SetInt(int64(integer))
		case reflect.Bool:
			boolean, err := strconv.ParseBool(content)
			if err != nil {
				log.Printf("Could not parse environment variable %s with value '%q' as boolean", prefix, content)
				return
			}
			value.SetBool(boolean)
		default:
			// ignore this field
			log.Printf("Setting configuration options of type %s via environment variables is not supported", value.Type().Name())
		}
	} else {
		for i := 0; i < value.NumField(); i++ {
			fieldName := value.Type().Field(i).Name
			newPrefix := fmt.Sprintf("%s_%s", prefix, strings.ToUpper(fieldName))
			readFromEnvironment(newPrefix, value.Field(i))
		}
	}
}
