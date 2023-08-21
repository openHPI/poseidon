package logging

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/sirupsen/logrus"
)

// ContextHook logs the values referenced by the of dto.LoggedContextKeys.
// By default Logrus does not log the values stored in the passed context.
type ContextHook struct{}

// Fire is triggered on new log entries.
func (hook *ContextHook) Fire(entry *logrus.Entry) error {
	if entry.Context != nil {
		injectContextValuesIntoData(entry)
	}
	return nil
}

func injectContextValuesIntoData(entry *logrus.Entry) {
	for _, key := range dto.LoggedContextKeys {
		value := entry.Context.Value(key)
		_, valueExisting := entry.Data[string(key)]
		if !valueExisting && value != nil {
			entry.Data[string(key)] = value
		}
	}
}

// Levels returns all levels this hook should be registered to.
func (hook *ContextHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}
}
