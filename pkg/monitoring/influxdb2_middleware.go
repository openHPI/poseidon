package monitoring

import (
	"bytes"
	"context"
	"github.com/gorilla/mux"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
	http2 "github.com/influxdata/influxdb-client-go/v2/api/http"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	// influxdbContextKey is a key (runner.ContextKey) to reference the influxdb data point in the request context.
	influxdbContextKey dto.ContextKey = "influxdb data point"
	// measurementPrefix allows easier filtering in influxdb.
	measurementPrefix           = "poseidon_"
	measurementPoolSize         = measurementPrefix + "poolsize"
	MeasurementNomadEvents      = measurementPrefix + "nomad_events"
	MeasurementNomadAllocations = measurementPrefix + "nomad_allocations"
	MeasurementIdleRunnerNomad  = measurementPrefix + "nomad_idle_runners"
	MeasurementExecutionsAWS    = measurementPrefix + "aws_executions"
	MeasurementExecutionsNomad  = measurementPrefix + "nomad_executions"
	MeasurementEnvironments     = measurementPrefix + "environments"
	MeasurementUsedRunner       = measurementPrefix + "used_runners"
	MeasurementFileDownload     = measurementPrefix + "file_download"

	// The keys for the monitored tags and fields.

	InfluxKeyRunnerID                      = dto.KeyRunnerID
	InfluxKeyEnvironmentID                 = dto.KeyEnvironmentID
	InfluxKeyJobID                         = "job_id"
	InfluxKeyClientStatus                  = "client_status"
	InfluxKeyNomadNode                     = "nomad_agent"
	InfluxKeyActualContentLength           = "actual_length"
	InfluxKeyExpectedContentLength         = "expected_length"
	InfluxKeyDuration                      = "duration"
	InfluxKeyStartupDuration               = "startup_" + InfluxKeyDuration
	influxKeyEnvironmentPrewarmingPoolSize = "prewarming_pool_size"
	influxKeyRequestSize                   = "request_size"
)

var (
	log          = logging.GetLogger("monitoring")
	influxClient influxdb2API.WriteAPI
)

func InitializeInfluxDB(db *config.InfluxDB) (cancel func()) {
	if db.URL == "" {
		return func() {}
	}

	// How long to wait before retrying to write data.
	const retryInterval = 30 * time.Second
	// How old the data can be before we stop retrying to write it. Should be larger than maxRetries * retryInterval.
	const retryExpire = 10 * time.Minute
	// How often to retry to write data.
	const maxRetries = 10

	// Set options for retrying with the influx client.
	options := influxdb2.DefaultOptions()
	options.SetRetryInterval(uint(retryInterval.Milliseconds()))
	options.SetMaxRetries(maxRetries)
	options.SetMaxRetryTime(uint(retryExpire.Milliseconds()))

	// Create a new influx client.
	client := influxdb2.NewClientWithOptions(db.URL, db.Token, options)
	influxClient = client.WriteAPI(db.Organization, db.Bucket)
	influxClient.SetWriteFailedCallback(func(_ string, error http2.Error, retryAttempts uint) bool {
		log.WithError(&error).WithField("retryAttempts", retryAttempts).Trace("Retrying to write influx data...")

		// retryAttempts means number of retries, 0 if it failed during first write.
		if retryAttempts == options.MaxRetries() {
			log.WithError(&error).Warn("Could not write influx data.")
			return false // Disable retry. We failed to retry writing the data in time.
		}
		return true // Enable retry (default)
	})

	// Flush the influx client on shutdown.
	cancel = func() {
		influxClient.Flush()
		influxClient = nil
		client.Close()
	}
	return cancel
}

// InfluxDB2Middleware is a middleware to send events to an influx database.
func InfluxDB2Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r).GetName()
		p := influxdb2.NewPointWithMeasurement(measurementPrefix + route)

		start := time.Now().UTC()
		p.SetTime(time.Now())

		ctx := context.WithValue(r.Context(), influxdbContextKey, p)
		requestWithPoint := r.WithContext(ctx)
		lrw := logging.NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, requestWithPoint)

		p.AddField(InfluxKeyDuration, time.Now().UTC().Sub(start).Nanoseconds())
		p.AddTag("status", strconv.Itoa(lrw.StatusCode))

		WriteInfluxPoint(p)
	})
}

// AddRunnerMonitoringData adds the data of the runner we want to monitor.
func AddRunnerMonitoringData(request *http.Request, runnerID string, environmentID dto.EnvironmentID) {
	addRunnerID(request, runnerID)
	addEnvironmentID(request, environmentID)
}

// addRunnerID adds the runner id to the influx data point for the current request.
func addRunnerID(r *http.Request, id string) {
	addInfluxDBTag(r, InfluxKeyRunnerID, id)
}

// addEnvironmentID adds the environment id to the influx data point for the current request.
func addEnvironmentID(r *http.Request, id dto.EnvironmentID) {
	addInfluxDBTag(r, InfluxKeyEnvironmentID, id.ToString())
}

// AddRequestSize adds the size of the request body to the influx data point for the current request.
func AddRequestSize(r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.WithContext(r.Context()).WithError(err).Debug("Failed to read request body")
		return
	}

	err = r.Body.Close()
	if err != nil {
		log.WithContext(r.Context()).WithError(err).Debug("Failed to close request body")
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	addInfluxDBField(r, influxKeyRequestSize, len(body))
}

func ChangedPrewarmingPoolSize(id dto.EnvironmentID, count uint) {
	p := influxdb2.NewPointWithMeasurement(measurementPoolSize)

	p.AddTag(InfluxKeyEnvironmentID, id.ToString())
	p.AddField(influxKeyEnvironmentPrewarmingPoolSize, count)

	WriteInfluxPoint(p)
}

// WriteInfluxPoint schedules the influx data point to be sent.
func WriteInfluxPoint(p *write.Point) {
	if influxClient != nil {
		p.AddTag("stage", config.Config.InfluxDB.Stage)
		influxClient.WritePoint(p)
	} else {
		entry := log.WithField("name", p.Name())
		for _, tag := range p.TagList() {
			if tag.Key == "event_type" && tag.Value == "periodically" {
				return
			}
			entry = entry.WithField(tag.Key, tag.Value)
		}
		for _, field := range p.FieldList() {
			entry = entry.WithField(field.Key, field.Value)
		}
		entry.Trace("Influx data point")
	}
}

// addInfluxDBTag adds a tag to the influxdb data point in the request.
func addInfluxDBTag(r *http.Request, key, value string) {
	dataPointFromRequest(r).AddTag(key, value)
}

// addInfluxDBField adds a field to the influxdb data point in the request.
func addInfluxDBField(r *http.Request, key string, value interface{}) {
	dataPointFromRequest(r).AddField(key, value)
}

// dataPointFromRequest returns the data point in the passed request.
func dataPointFromRequest(r *http.Request) *write.Point {
	p, ok := r.Context().Value(influxdbContextKey).(*write.Point)
	if !ok {
		log.WithContext(r.Context()).Error("All http request must contain an influxdb data point!")
	}
	return p
}
