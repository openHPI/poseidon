package nullio

import (
	"errors"
	"fmt"
	"net/http"
)

var ErrRegexMatching = errors.New("could not match content length")

// ContentLengthWriter implements io.Writer.
// It parses the size from the first line as Content Length Header and streams the following data to the Target.
// The first line is expected to follow the format headerLineRegex.
type ContentLengthWriter struct {
	Target           http.ResponseWriter
	contentLengthSet bool
	firstLine        []byte
}

func (w *ContentLengthWriter) Write(p []byte) (count int, err error) {
	if w.contentLengthSet {
		count, err = w.Target.Write(p)
		if err != nil {
			err = fmt.Errorf("could not write to target: %w", err)
		}
		return count, err
	}

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
