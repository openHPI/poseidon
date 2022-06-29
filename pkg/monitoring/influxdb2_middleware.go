package monitoring

import (
	"bytes"
	"context"
	"github.com/gorilla/mux"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
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
	measurementPrefix          = "poseidon_"
	measurementPoolSize        = measurementPrefix + "poolsize"
	MeasurementIdleRunnerNomad = measurementPrefix + "nomad_idle_runners"
	MeasurementExecutionsAWS   = measurementPrefix + "aws_executions"
	MeasurementExecutionsNomad = measurementPrefix + "nomad_executions"
	MeasurementEnvironments    = measurementPrefix + "environments"
	MeasurementUsedRunner      = measurementPrefix + "used_runners"

	// The keys for the monitored tags and fields.
	influxKeyRunnerID                      = "runner_id"
	influxKeyEnvironmentID                 = "environment_id"
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

	client := influxdb2.NewClient(db.URL, db.Token)
	influxClient = client.WriteAPI(db.Organization, db.Bucket)
	cancel = func() {
		influxClient.Flush()
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

		p.AddField("duration", time.Now().UTC().Sub(start).Nanoseconds())
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
	addInfluxDBTag(r, influxKeyRunnerID, id)
}

// addEnvironmentID adds the environment id to the influx data point for the current request.
func addEnvironmentID(r *http.Request, id dto.EnvironmentID) {
	addInfluxDBTag(r, influxKeyEnvironmentID, strconv.Itoa(int(id)))
}

// AddRequestSize adds the size of the request body to the influx data point for the current request.
func AddRequestSize(r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Warn("Failed to read request body")
	}

	err = r.Body.Close()
	if err != nil {
		log.WithError(err).Warn("Failed to close request body")
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	addInfluxDBField(r, influxKeyRequestSize, len(body))
}

func ChangedPrewarmingPoolSize(id dto.EnvironmentID, count uint) {
	p := influxdb2.NewPointWithMeasurement(measurementPoolSize)

	p.AddTag(influxKeyEnvironmentID, strconv.Itoa(int(id)))
	p.AddField(influxKeyEnvironmentPrewarmingPoolSize, count)

	WriteInfluxPoint(p)
}

// WriteInfluxPoint schedules the indlux data point to be sent.
func WriteInfluxPoint(p *write.Point) {
	if influxClient != nil {
		p.AddTag("stage", config.Config.InfluxDB.Stage)
		influxClient.WritePoint(p)
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
		log.Error("All http request must contain an influxdb data point!")
	}
	return p
}
