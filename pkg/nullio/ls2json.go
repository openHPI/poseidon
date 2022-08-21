package nullio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"io"
	"regexp"
	"strconv"
)

var (
	log             = logging.GetLogger("nullio")
	pathLineRegex   = regexp.MustCompile(`(.*):$`)
	headerLineRegex = regexp.MustCompile(`([dl-])[-rwxXsS]{9} \d* .*? .*? +(\d+) (\d+) (.*)$`)
)

// Ls2JsonWriter implements io.Writer.
// It streams the passed data to the Target and transforms the data into the json format.
type Ls2JsonWriter struct {
	Target         io.Writer
	jsonStartSend  bool
	setCommaPrefix bool
	remaining      []byte
	latestPath     []byte
}

func (w *Ls2JsonWriter) Write(p []byte) (int, error) {
	i, err := w.initializeJSONObject()
	if err != nil {
		return i, err
	}

	start := 0
	for i, char := range p {
		if char != '\n' {
			continue
		}

		line := p[start:i]
		if len(w.remaining) > 0 {
			line = append(w.remaining, line...)
			w.remaining = []byte("")
		}

		if len(line) != 0 {
			count, err := w.writeLine(line)
			if err != nil {
				log.WithError(err).Warn("Could not write line to Target")
				return count, err
			}
		}
		start = i + 1
	}

	if start < len(p) {
		w.remaining = p[start:]
	}

	return len(p), nil
}

func (w *Ls2JsonWriter) initializeJSONObject() (count int, err error) {
	if !w.jsonStartSend {
		count, err = w.Target.Write([]byte("{\"files\": ["))
		if count == 0 || err != nil {
			log.WithError(err).Warn("Could not write to target")
			err = fmt.Errorf("could not write to target: %w", err)
		} else {
			w.jsonStartSend = true
		}
	}
	return count, err
}

func (w *Ls2JsonWriter) Close() {
	if w.jsonStartSend {
		count, err := w.Target.Write([]byte("]}"))
		if count == 0 || err != nil {
			log.WithError(err).Warn("Could not Close ls2json writer")
		}
	}
}

func (w *Ls2JsonWriter) writeLine(line []byte) (count int, err error) {
	matches := pathLineRegex.FindSubmatch(line)
	if matches != nil {
		w.latestPath = append(bytes.TrimSuffix(matches[1], []byte("/")), '/')
		return 0, nil
	}

	matches = headerLineRegex.FindSubmatch(line)
	if matches != nil {
		size, err1 := strconv.Atoi(string(matches[2]))
		timestamp, err2 := strconv.Atoi(string(matches[3]))
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("could not parse file details: %w %+v", err1, err2)
		}

		response, err1 := json.Marshal(dto.FileHeader{
			Name:             dto.FilePath(append(w.latestPath, matches[4]...)),
			ObjectType:       string(matches[1][0]),
			Size:             size,
			ModificationTime: timestamp,
		})
		if err1 != nil {
			return 0, fmt.Errorf("could not marshal file header: %w", err)
		}

		// Skip the first leading comma
		if w.setCommaPrefix {
			response = append([]byte{','}, response...)
		} else {
			w.setCommaPrefix = true
		}

		count, err1 = w.Target.Write(response)
		if err1 != nil {
			err = fmt.Errorf("could not write to target: %w", err)
		} else if count == len(response) {
			count = len(line)
		}
	}

	return count, err
}
