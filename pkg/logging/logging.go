package logging

import (
	"bufio"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const TimestampFormat = "2006-01-02T15:04:05.000000Z"

var log = &logrus.Logger{
	Out: os.Stderr,
	Formatter: &logrus.TextFormatter{
		TimestampFormat: TimestampFormat,
		DisableColors:   true,
		FullTimestamp:   true,
	},
	Hooks: make(logrus.LevelHooks),
	Level: logrus.InfoLevel,
}

const GracefulSentryShutdown = 5 * time.Second

func InitializeLogging(logLevel string, formatter dto.Formatter) {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Fatal("Error parsing loglevel")
		return
	}
	log.SetLevel(level)
	if formatter == dto.FormatterJSON {
		log.Formatter = &logrus.JSONFormatter{
			TimestampFormat: TimestampFormat,
		}
	}
	log.AddHook(&SentryHook{})
	log.ExitFunc = func(i int) {
		sentry.Flush(GracefulSentryShutdown)
		os.Exit(i)
	}
}

func GetLogger(pkg string) *logrus.Entry {
	return log.WithField("package", pkg)
}

// ResponseWriter wraps the default http.ResponseWriter and catches the status code
// that is written.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{w, http.StatusOK}
}

func (writer *ResponseWriter) WriteHeader(code int) {
	writer.StatusCode = code
	writer.ResponseWriter.WriteHeader(code)
}

func (writer *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := writer.ResponseWriter.(http.Hijacker).Hijack()
	if err != nil {
		return conn, nil, fmt.Errorf("hijacking connection failed: %w", err)
	}
	return conn, rw, nil
}

// HTTPLoggingMiddleware returns a http.Handler that logs different information about every request.
func HTTPLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		path := RemoveNewlineSymbol(r.URL.Path)

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		latency := time.Now().UTC().Sub(start)
		logEntry := log.WithContext(r.Context()).WithFields(logrus.Fields{
			"code":       lrw.StatusCode,
			"method":     r.Method,
			"path":       path,
			"duration":   latency,
			"user_agent": RemoveNewlineSymbol(r.UserAgent()),
		})
		if lrw.StatusCode >= http.StatusInternalServerError {
			logEntry.Error("Failing " + path)
		} else {
			logEntry.Debug()
		}
	})
}

// RemoveNewlineSymbol GOOD: remove newlines from user controlled input before logging.
func RemoveNewlineSymbol(data string) string {
	data = strings.ReplaceAll(data, "\r", "")
	data = strings.ReplaceAll(data, "\n", "")
	return data
}
