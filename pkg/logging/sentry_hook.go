package logging

import (
	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
)

// SentryHook is a simple adapter that converts logrus entries into Sentry events.
// Consider replacing this with a more feature rich, additional dependency: https://github.com/evalphobia/logrus_sentry
type SentryHook struct{}

// Fire is triggered on new log entries.
func (hook *SentryHook) Fire(entry *logrus.Entry) error {
	if data, ok := entry.Data["error"]; ok {
		err, ok := data.(error)
		if ok {
			entry.Data["error"] = err.Error()
		}
	}

	event := sentry.NewEvent()
	event.Timestamp = entry.Time
	event.Level = sentry.Level(entry.Level.String())
	event.Message = entry.Message
	event.Extra = entry.Data
	sentry.CaptureEvent(event)
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
