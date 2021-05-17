package logging

import (
	"bufio"
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"time"
)

var log = &logrus.Logger{
	Out: os.Stderr,
	Formatter: &logrus.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	},
	Hooks: make(logrus.LevelHooks),
	Level: logrus.InfoLevel,
}

func InitializeLogging(loglevel string) {
	level, err := logrus.ParseLevel(loglevel)
	if err != nil {
		log.WithError(err).Fatal("Error parsing loglevel")
		return
	}
	log.SetLevel(level)
}

func GetLogger(pkg string) *logrus.Entry {
	return log.WithField("package", pkg)
}

// loggingResponseWriter wraps the default http.ResponseWriter and catches the status code
// that is written
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (writer *loggingResponseWriter) WriteHeader(code int) {
	writer.statusCode = code
	writer.ResponseWriter.WriteHeader(code)
}

func (writer *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return writer.ResponseWriter.(http.Hijacker).Hijack()
}

// HTTPLoggingMiddleware returns an http.Handler that logs different information about every request
func HTTPLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		path := r.URL.Path

		lrw := NewLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		latency := time.Now().UTC().Sub(start)
		logEntry := log.WithFields(logrus.Fields{
			"code":       lrw.statusCode,
			"method":     r.Method,
			"path":       path,
			"duration":   latency,
			"user_agent": r.UserAgent(),
		})
		if lrw.statusCode >= http.StatusInternalServerError {
			logEntry.Warn()
		} else {
			logEntry.Debug()
		}
	})
}
