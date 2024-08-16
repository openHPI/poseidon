package logging

import (
	"context"
	"errors"

	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/sirupsen/logrus"
)

// SentryHook is a simple adapter that converts logrus entries into Sentry events.
// Consider replacing this with a more feature rich, additional dependency: https://github.com/evalphobia/logrus_sentry
type SentryHook struct{}

const (
	SentryContextKey          = "Poseidon Details"
	SentryFingerprintFieldKey = "sentry-fingerprint"
)

var ErrHubInvalid = errors.New("the hub is invalid")

// Fire is triggered on new log entries.
func (hook *SentryHook) Fire(entry *logrus.Entry) (err error) {
	hub := handleEntryContext(entry)
	hub.WithScope(func(scope *sentry.Scope) {
		err = handleLogEntry(entry, hub, scope)
	})
	return err
}

func handleEntryContext(entry *logrus.Entry) *sentry.Hub {
	var hub *sentry.Hub
	if entry.Context != nil {
		hub = sentry.GetHubFromContext(entry.Context)
		injectContextValuesIntoData(entry)
	}
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	return hub
}

func handleLogEntry(entry *logrus.Entry, hub *sentry.Hub, scope *sentry.Scope) (err error) {
	client := hub.Client()
	if client == nil || scope == nil {
		return ErrHubInvalid
	}

	scope.SetContext(SentryContextKey, entry.Data)
	if runnerID, ok := entry.Data[dto.KeyRunnerID].(string); ok {
		scope.SetTag(dto.KeyRunnerID, runnerID)
	}
	if environmentID, ok := entry.Data[dto.KeyEnvironmentID].(string); ok {
		scope.SetTag(dto.KeyEnvironmentID, environmentID)
	}
	if fingerprint, ok := entry.Data[SentryFingerprintFieldKey].(string); ok {
		scope.SetFingerprint([]string{fingerprint})
	}

	event := client.EventFromMessage(entry.Message, sentry.Level(entry.Level.String()))
	event.Timestamp = entry.Time
	if data, ok := entry.Data["error"]; ok {
		entryError, ok := data.(error)
		if ok {
			entry.Data["error"] = entryError.Error()
		}
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

func StartSpan(ctx context.Context, op, description string, callback func(context.Context)) {
	span := sentry.StartSpan(ctx, op)
	span.Description = description
	defer span.Finish()
	callback(ctx)
}
