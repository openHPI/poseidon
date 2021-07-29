package config

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/logging"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Config contains the default configuration of Poseidon.
var (
	Config = &configuration{
		Server: server{
			Address:           "127.0.0.1",
			Port:              7200,
			Token:             "",
			TLS:               false,
			CertFile:          "",
			KeyFile:           "",
			InteractiveStderr: true,
		},
		Nomad: nomad{
			Address:   "127.0.0.1",
			Port:      4646,
			Token:     "",
			TLS:       false,
			Namespace: "default",
		},
		Logger: logger{
			Level: "INFO",
		},
	}
	configurationFilePath    = "./configuration.yaml"
	configurationInitialized = false
	log                      = logging.GetLogger("config")
	TLSConfig                = &tls.Config{
		MinVersion:               tls.VersionTLS13,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
	}
	ErrConfigInitialized = errors.New("configuration is already initialized")
)

// server configures the Poseidon webserver.
type server struct {
	Address           string
	Port              int
	Token             string
	TLS               bool
	CertFile          string
	KeyFile           string
	InteractiveStderr bool
}

// nomad configures the used Nomad cluster.
type nomad struct {
	Address   string
	Port      int
	Token     string
	TLS       bool
	Namespace string
}

// logger configures the used logger.
type logger struct {
	Level string
}

// configuration contains the complete configuration of Poseidon.
type configuration struct {
	Server server
	Nomad  nomad
	Logger logger
}

// InitConfig merges configuration options from environment variables and
// a configuration file into the default configuration. Calls of InitConfig
// after the first call have no effect and return an error. InitConfig
// should be called directly after starting the program.
func InitConfig() error {
	if configurationInitialized {
		return ErrConfigInitialized
	}
	configurationInitialized = true
	content := readConfigFile()
	Config.mergeYaml(content)
	Config.mergeEnvironmentVariables()
	return nil
}

// NomadAPIURL returns the URL for the configured Nomad cluster.
func (c *configuration) NomadAPIURL() *url.URL {
	return parseURL(Config.Nomad.Address, Config.Nomad.Port, Config.Nomad.TLS)
}

// PoseidonAPIURL returns the URL of the Poseidon webserver.
func (c *configuration) PoseidonAPIURL() *url.URL {
	return parseURL(Config.Server.Address, Config.Server.Port, false)
}

func parseURL(address string, port int, tlsEnabled bool) *url.URL {
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	return &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", address, port),
	}
}

func readConfigFile() []byte {
	parseFlags()
	data, err := os.ReadFile(configurationFilePath)
	if err != nil {
		log.WithError(err).Info("Using default configuration...")
		return nil
	}
	return data
}

func parseFlags() {
	if flag.Lookup("config") == nil {
		flag.StringVar(&configurationFilePath, "config", configurationFilePath, "path of the yaml config file")
	}
	flag.Parse()
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
	logEntry := log.WithField("prefix", prefix)
	// if value was not derived from a pointer, it is not possible to alter its contents
	if !value.CanSet() {
		logEntry.Warn("Cannot overwrite struct field that can not be set")
		return
	}

	if value.Kind() != reflect.Struct {
		loadValue(prefix, value, logEntry)
	} else {
		for i := 0; i < value.NumField(); i++ {
			fieldName := value.Type().Field(i).Name
			newPrefix := fmt.Sprintf("%s_%s", prefix, strings.ToUpper(fieldName))
			readFromEnvironment(newPrefix, value.Field(i))
		}
	}
}

func loadValue(prefix string, value reflect.Value, logEntry *logrus.Entry) {
	content, ok := os.LookupEnv(prefix)
	if !ok {
		return
	}
	logEntry = logEntry.WithField("content", content)

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
		logEntry.WithField("type", value.Type().Name()).
			Warn("Setting configuration option via environment variables is not supported")
	}
}