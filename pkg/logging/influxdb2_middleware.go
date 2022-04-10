package logging

import (
	"bytes"
	"context"
	"github.com/gorilla/mux"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/dto"
	"io"
	"net/http"
	"strconv"
	"time"
)

// InfluxdbContextKey is a key to reference the influxdb data point in the request context.
const InfluxdbContextKey = "influxdb data point"

// InfluxdbMeasurementPrefix allows easier filtering in influxdb.
const InfluxdbMeasurementPrefix = "poseidon_"

// InfluxDB2Middleware is a middleware to send events to an influx database.
func InfluxDB2Middleware(influxClient influxdb2API.WriteAPI) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := mux.CurrentRoute(r).GetName()
			p := influxdb2.NewPointWithMeasurement(InfluxdbMeasurementPrefix + route)

			start := time.Now().UTC()
			p.SetTime(time.Now())

			requestWithPoint := r.WithContext(newContextWithPoint(r.Context(), p))
			lrw := NewLoggingResponseWriter(w)
			next.ServeHTTP(lrw, requestWithPoint)

			p.AddField("duration", time.Now().UTC().Sub(start).Nanoseconds())
			p.AddTag("status", strconv.Itoa(lrw.statusCode))

			if influxClient != nil {
				influxClient.WritePoint(p)
			}
		})
	}
}

// AddEnvironmentID adds the environment id to the influx data point for the current request.
func AddEnvironmentID(r *http.Request, id dto.EnvironmentID) {
	p := pointFromContext(r.Context())
	p.AddTag("environment_id", strconv.Itoa(int(id)))
}

// AddEnvironmentType adds the type of the used environment to the influxdb data point.
func AddEnvironmentType(r *http.Request, t string) {
	p := pointFromContext(r.Context())
	p.AddTag("environment_type", t)
}

// AddRunnerID adds the runner id to the influx data point for the current request.
func AddRunnerID(r *http.Request, id string) {
	p := pointFromContext(r.Context())
	p.AddTag("runner_id", id)
}

// AddIdleRunner adds the count of idle runners of the used environment to the influxdb data point.
func AddIdleRunner(r *http.Request, count int) {
	p := pointFromContext(r.Context())
	p.AddField("idle_runner", strconv.Itoa(count))
}

// AddPrewarmingPoolSize adds the prewarming pool size of the used environment to the influxdb data point.
func AddPrewarmingPoolSize(r *http.Request, count uint) {
	p := pointFromContext(r.Context())
	p.AddField("prewarming_pool_size", strconv.Itoa(int(count)))
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

	p := pointFromContext(r.Context())
	p.AddField("request_size", strconv.Itoa(len(body)))
}

// newContextWithPoint creates a context containing an InfluxDB data point.
func newContextWithPoint(ctx context.Context, p *write.Point) context.Context {
	return context.WithValue(ctx, InfluxdbContextKey, p)
}

// pointFromContext returns an InfluxDB data point from a context.
func pointFromContext(ctx context.Context) *write.Point {
	p, ok := ctx.Value(InfluxdbContextKey).(*write.Point)
	if !ok {
		log.Errorf("InfluxDB data point not stored in context.")
	}
	return p
}
