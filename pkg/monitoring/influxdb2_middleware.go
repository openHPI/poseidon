package monitoring

import (
	"bytes"
	"context"
	"github.com/gorilla/mux"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

var log = logging.GetLogger("monitoring")

const (
	// influxdbContextKey is a key to reference the influxdb data point in the request context.
	influxdbContextKey runner.ContextKey = "influxdb data point"
	// influxdbMeasurementPrefix allows easier filtering in influxdb.
	influxdbMeasurementPrefix = "poseidon_"

	// The keys for the monitored tags and fields.
	influxKeyRunnerID                      = "runner_id"
	influxKeyEnvironmentID                 = "environment_id"
	influxKeyEnvironmentType               = "environment_type"
	influxKeyEnvironmentIdleRunner         = "idle_runner"
	influxKeyEnvironmentPrewarmingPoolSize = "prewarming_pool_size"
	influxKeyRequestSize                   = "request_size"
)

// InfluxDB2Middleware is a middleware to send events to an influx database.
func InfluxDB2Middleware(influxClient influxdb2API.WriteAPI, manager environment.Manager) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := mux.CurrentRoute(r).GetName()
			p := influxdb2.NewPointWithMeasurement(influxdbMeasurementPrefix + route)
			p.AddTag("stage", config.Config.InfluxDB.Stage)

			start := time.Now().UTC()
			p.SetTime(time.Now())

			ctx := context.WithValue(r.Context(), influxdbContextKey, p)
			requestWithPoint := r.WithContext(ctx)
			lrw := logging.NewLoggingResponseWriter(w)
			next.ServeHTTP(lrw, requestWithPoint)

			p.AddField("duration", time.Now().UTC().Sub(start).Nanoseconds())
			p.AddTag("status", strconv.Itoa(lrw.StatusCode))

			environmentID, err := strconv.Atoi(getEnvironmentID(p))
			if err == nil && manager != nil {
				addEnvironmentData(p, manager, dto.EnvironmentID(environmentID))
			}

			if influxClient != nil {
				influxClient.WritePoint(p)
			}
		})
	}
}

// AddRunnerMonitoringData adds the data of the runner we want to monitor.
func AddRunnerMonitoringData(request *http.Request, r runner.Runner) {
	addRunnerID(request, r.ID())
	addEnvironmentID(request, r.Environment())
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

// getEnvironmentID tries to find the environment id in the influxdb data point.
func getEnvironmentID(p *write.Point) string {
	for _, tag := range p.TagList() {
		if tag.Key == influxKeyEnvironmentID {
			return tag.Value
		}
	}
	return ""
}

// addEnvironmentData adds environment specific data to the influxdb data point.
func addEnvironmentData(p *write.Point, manager environment.Manager, id dto.EnvironmentID) {
	e, err := manager.Get(id, false)
	if err == nil {
		p.AddTag(influxKeyEnvironmentType, getType(e))
		p.AddField(influxKeyEnvironmentIdleRunner, e.IdleRunnerCount())
		p.AddField(influxKeyEnvironmentPrewarmingPoolSize, e.PrewarmingPoolSize())
	}
}

// Get type returns the type of the passed execution environment.
func getType(e runner.ExecutionEnvironment) string {
	if t := reflect.TypeOf(e); t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	} else {
		return t.Name()
	}
}
