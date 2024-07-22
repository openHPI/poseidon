package config

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/getsentry/sentry-go"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	defaultPoseidonPort              = 7200
	defaultNomadPort                 = 4646
	defaultMemoryUsageAlertThreshold = 1_000
)

// Config contains the default configuration of Poseidon.
var (
	Config = &configuration{
		Server: server{
			Address:                 "127.0.0.1",
			Port:                    defaultPoseidonPort,
			SystemdSocketActivation: false,
			Token:                   "",
			TLS: TLS{
				Active:   false,
				CAFile:   "",
				CertFile: "",
				KeyFile:  "",
			},
			InteractiveStderr: true,
			TemplateJobFile:   "",
			Alert: alert{
				PrewarmingPoolThreshold:     0,
				PrewarmingPoolReloadTimeout: 0,
			},
			LoggingFilterToken: randomFilterToken(),
		},
		Nomad: Nomad{
			Enabled: true,
			Address: "127.0.0.1",
			Port:    defaultNomadPort,
			Token:   "",
			TLS: TLS{
				Active:   false,
				CAFile:   "",
				CertFile: "",
				KeyFile:  "",
			},
			Namespace:        "default",
			DisableForcePull: false,
			Network: nomadApi.NetworkResource{
				Mode: "bridge",
				DNS:  nil,
			},
		},
		AWS: AWS{
			Enabled:   false,
			Endpoint:  "",
			Functions: []string{},
		},
		Logger: Logger{
			Level:     "INFO",
			Formatter: dto.FormatterText,
		},
		Profiling: Profiling{
			MemoryThreshold: defaultMemoryUsageAlertThreshold,
		},
		Sentry: sentry.ClientOptions{
			AttachStacktrace: true,
		},
		InfluxDB: InfluxDB{
			URL:          "",
			Token:        "",
			Organization: "",
			Bucket:       "",
			Stage:        "",
		},
	}
	configurationFilePath    = "./configuration.yaml"
	configurationInitialized = false
	log                      = logging.GetLogger("config")
	TLSConfig                = &tls.Config{
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
	}
	ErrConfigInitialized = errors.New("configuration is already initialized")
)

type alert struct {
	PrewarmingPoolThreshold     float64 `yaml:"prewarmingpoolthreshold"`
	PrewarmingPoolReloadTimeout uint    `yaml:"prewarmingpoolreloadtimeout"`
}

// server configures the Poseidon webserver.
type server struct {
	Address                 string `yaml:"address"`
	Port                    int    `yaml:"port"`
	SystemdSocketActivation bool   `yaml:"systemdsocketactivation"`
	Token                   string `yaml:"token"`
	TLS                     TLS    `yaml:"tls"`
	InteractiveStderr       bool   `yaml:"interactivestderr"`
	TemplateJobFile         string `yaml:"templatejobfile"`
	Alert                   alert  `yaml:"alert"`
	LoggingFilterToken      string `yaml:"loggingfiltertoken"`
}

// URL returns the URL of the Poseidon webserver.
func (s *server) URL() *url.URL {
	return parseURL(s.Address, s.Port, s.TLS.Active)
}

// Nomad configures the used Nomad cluster.
type Nomad struct {
	Enabled          bool                     `yaml:"enabled"`
	Address          string                   `yaml:"address"`
	Port             int                      `yaml:"port"`
	Token            string                   `yaml:"token"`
	TLS              TLS                      `yaml:"tls"`
	Namespace        string                   `yaml:"namespace"`
	DisableForcePull bool                     `yaml:"disableforcepull"`
	Network          nomadApi.NetworkResource `yaml:"network"`
}

// URL returns the URL for the configured Nomad cluster.
func (n *Nomad) URL() *url.URL {
	return parseURL(n.Address, n.Port, n.TLS.Active)
}

// AWS configures the AWS Lambda usage.
type AWS struct {
	Enabled   bool     `yaml:"enabled"`
	Endpoint  string   `yaml:"endpoint"`
	Functions []string `yaml:"functions"`
}

// TLS configures TLS on a connection.
type TLS struct {
	Active   bool   `yaml:"active"`
	CAFile   string `yaml:"cafile"`
	CertFile string `yaml:"certfile"`
	KeyFile  string `yaml:"keyfile"`
}

// Logger configures the used Logger.
type Logger struct {
	Formatter dto.Formatter `yaml:"formatter"`
	Level     string        `yaml:"level"`
}

// Profiling configures the usage of a runtime profiler to create optimized binaries.
type Profiling struct {
	CPUEnabled      bool   `yaml:"cpuenabled"`
	CPUFile         string `yaml:"cpufile"`
	MemoryInterval  uint   `yaml:"memoryinterval"`
	MemoryThreshold uint   `yaml:"memorythreshold"`
}

// InfluxDB configures the usage of an Influx db monitoring.
type InfluxDB struct {
	URL          string `yaml:"url"`
	Token        string `yaml:"token"`
	Organization string `yaml:"organization"`
	Bucket       string `yaml:"bucket"`
	Stage        string `yaml:"stage"`
}

// configuration contains the complete configuration of Poseidon.
type configuration struct {
	Server    server               `yaml:"server"`
	Nomad     Nomad                `yaml:"nomad"`
	AWS       AWS                  `yaml:"aws"`
	Logger    Logger               `yaml:"logger"`
	Profiling Profiling            `yaml:"profiling"`
	Sentry    sentry.ClientOptions `yaml:"sentry"`
	InfluxDB  InfluxDB             `yaml:"influxdb"`
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
		for i := range value.NumField() {
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
	case reflect.Slice:
		if content != "" && content[0] == '"' && content[len(content)-1] == '"' {
			content = content[1 : len(content)-1] // remove wrapping quotes
		}
		parts := strings.Fields(content)
		value.Set(reflect.AppendSlice(value, reflect.ValueOf(parts)))
	default:
		// ignore this field
		logEntry.WithField("type", value.Type().Name()).
			Warn("Setting configuration option via environment variables is not supported")
	}
}

func randomFilterToken() string {
	const tokenLength = 32
	randomBytes := make([]byte, tokenLength)
	n, err := rand.Read(randomBytes)
	if n != tokenLength || err != nil {
		log.WithError(err).WithField("byteCount", n).Fatal("Failed to generate random token")
	}

	return base64.URLEncoding.EncodeToString(randomBytes)
}
