package nullio

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/monitoring"
)

var ErrRegexMatching = errors.New("could not match content length")

// ContentLengthWriter implements io.Writer.
// It parses the size from the first line as Content Length Header and streams the following data to the Target.
// The first line is expected to follow the format headerLineRegex.
type ContentLengthWriter struct {
	Target                http.ResponseWriter
	contentLengthSet      bool
	firstLine             []byte
	expectedContentLength int
	actualContentLength   int
}

func (w *ContentLengthWriter) Write(p []byte) (count int, err error) {
	if w.contentLengthSet {
		return w.handleDataForwarding(p)
	} else {
		return w.handleContentLengthParsing(p)
	}
}

func (w *ContentLengthWriter) handleDataForwarding(p []byte) (int, error) {
	count, err := w.Target.Write(p)
	if err != nil {
		err = fmt.Errorf("could not write to target: %w", err)
	}
	w.actualContentLength += count
	return count, err
}

func (w *ContentLengthWriter) handleContentLengthParsing(p []byte) (count int, err error) {
	for i, char := range p {
		if char != '\n' {
			continue
		}

		w.firstLine = append(w.firstLine, p[:i]...)
		matches := headerLineRegex.FindSubmatch(w.firstLine)
		if len(matches) < headerLineGroupName {
			log.WithField("line", string(w.firstLine)).Error(ErrRegexMatching.Error())
			return 0, ErrRegexMatching
		}
		size := string(matches[headerLineGroupSize])
		w.expectedContentLength, err = strconv.Atoi(size)
		if err != nil {
			log.WithField("size", size).Warn("could not parse content length")
		}
		w.Target.Header().Set("Content-Length", size)
		w.contentLengthSet = true

		if i < len(p)-1 {
			count, err = w.Target.Write(p[i+1:])
			if err != nil {
				err = fmt.Errorf("could not write to target: %w", err)
			}
		}

		return len(p[:i]) + 1 + count, err
	}

	if !w.contentLengthSet {
		w.firstLine = append(w.firstLine, p...)
	}

	return len(p), nil
}

// SendMonitoringData will send a monitoring event of the content length read and written.
func (w *ContentLengthWriter) SendMonitoringData(p *write.Point) {
	p.AddField(monitoring.InfluxKeyExpectedContentLength, w.expectedContentLength)
	p.AddField(monitoring.InfluxKeyActualContentLength, w.actualContentLength)
	monitoring.WriteInfluxPoint(p)
}
