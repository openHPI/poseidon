package logging

import (
	"context"
	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/sirupsen/logrus"
)

// SentryHook is a simple adapter that converts logrus entries into Sentry events.
// Consider replacing this with a more feature rich, additional dependency: https://github.com/evalphobia/logrus_sentry
type SentryHook struct{}

// Fire is triggered on new log entries.
func (hook *SentryHook) Fire(entry *logrus.Entry) error {
	event := sentry.NewEvent()
	event.Timestamp = entry.Time
	event.Level = sentry.Level(entry.Level.String())
	event.Message = entry.Message

	// Add Stack Trace when an error was passed.
	if data, ok := entry.Data["error"]; ok {
		err, ok := data.(error)
		if ok {
			const maxErrorDepth = 10
			event.SetException(err, maxErrorDepth)
			entry.Data["error"] = err.Error()
		}
	}

	var hub *sentry.Hub
	if entry.Context != nil {
		hub = sentry.GetHubFromContext(entry.Context)
		// This might overwrite valid data when not the request context is passed.
		entry.Data[dto.KeyRunnerID] = entry.Context.Value(dto.ContextKey(dto.KeyRunnerID))
		entry.Data[dto.KeyEnvironmentID] = entry.Context.Value(dto.ContextKey(dto.KeyEnvironmentID))
	}
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	hub.Scope().SetContext("Poseidon Details", entry.Data)
	if runnerID, ok := entry.Data[dto.KeyRunnerID].(string); ok {
		hub.Scope().SetTag(dto.KeyRunnerID, runnerID)
	}
	if environmentID, ok := entry.Data[dto.KeyEnvironmentID].(string); ok {
		hub.Scope().SetTag(dto.KeyEnvironmentID, environmentID)
	}

	hub.CaptureEvent(event)
	return nil
}

// Levels returns all levels this hook should be registered to.
func (hook *SentryHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
	}
}

func StartSpan(op, description string, ctx context.Context, callback func(context.Context)) {
	span := sentry.StartSpan(ctx, op)
	span.Description = description
	defer span.Finish()
	callback(span.Context())
}
