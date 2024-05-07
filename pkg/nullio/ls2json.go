package nullio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
)

var (
	log             = logging.GetLogger("nullio")
	pathLineRegex   = regexp.MustCompile(`(.*):$`)
	headerLineRegex = regexp.
			MustCompile(`([-aAbcCdDlMnpPsw?])([-rwxXsStT]{9})(\+?) +\d+ +(.+?) +(.+?) +(\d+) +(\d+) +(.*)$`)
)

const (
	headerLineGroupEntryType   = 1
	headerLineGroupPermissions = 2
	headerLineGroupACL         = 3
	headerLineGroupOwner       = 4
	headerLineGroupGroup       = 5
	headerLineGroupSize        = 6
	headerLineGroupTimestamp   = 7
	headerLineGroupName        = 8
)

// Ls2JsonWriter implements io.Writer.
// It streams the passed data to the Target and transforms the data into the json format.
type Ls2JsonWriter struct {
	Target         io.Writer
	Ctx            context.Context
	jsonStartSent  bool
	setCommaPrefix bool
	remaining      []byte
	latestPath     []byte
	sentrySpan     *sentry.Span
}

func (w *Ls2JsonWriter) HasStartedWriting() bool {
	return w.jsonStartSent
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
				log.WithContext(w.Ctx).WithError(err).Warn("Could not write line to Target")
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
	if !w.jsonStartSent {
		count, err = w.Target.Write([]byte("{\"files\": ["))
		if count == 0 || err != nil {
			log.WithContext(w.Ctx).WithError(err).Warn("Could not write to target")
			err = fmt.Errorf("could not write to target: %w", err)
		} else {
			w.jsonStartSent = true
			w.sentrySpan = sentry.StartSpan(w.Ctx, "nullio.init")
			w.sentrySpan.Description = "Forwarding"
		}
	}
	return count, err
}

func (w *Ls2JsonWriter) Close() {
	if w.jsonStartSent {
		count, err := w.Target.Write([]byte("]}"))
		if count == 0 || err != nil {
			log.WithContext(w.Ctx).WithError(err).Warn("Could not Close ls2json writer")
		}
		w.sentrySpan.Finish()
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
		response, err1 := w.parseFileHeader(matches)
		if err1 != nil {
			return 0, err1
		}

		// Skip the first leading comma
		if w.setCommaPrefix {
			response = append([]byte{','}, response...)
		} else {
			w.setCommaPrefix = true
		}

		count, err1 = w.Target.Write(response)
		if err1 != nil {
			err = fmt.Errorf("could not write to target: %w", err1)
		} else if count == len(response) {
			count = len(line)
		}
	}

	return count, err
}

func (w *Ls2JsonWriter) parseFileHeader(matches [][]byte) ([]byte, error) {
	entryType := dto.EntryType(matches[headerLineGroupEntryType][0])
	permissions := string(matches[headerLineGroupPermissions])
	acl := string(matches[headerLineGroupACL])
	if acl == "+" {
		permissions += "+"
	}

	size, err1 := strconv.Atoi(string(matches[headerLineGroupSize]))
	timestamp, err2 := strconv.Atoi(string(matches[headerLineGroupTimestamp]))
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("could not parse file details: %w %+v", err1, err2)
	}

	name := dto.FilePath(append(w.latestPath, matches[headerLineGroupName]...))
	linkTarget := dto.FilePath("")
	if entryType == dto.EntryTypeLink {
		parts := strings.Split(string(name), " -> ")
		const NumberOfPartsInALink = 2
		if len(parts) == NumberOfPartsInALink {
			name = dto.FilePath(parts[0])
			linkTarget = dto.FilePath(parts[1])
		} else {
			log.WithContext(w.Ctx).Error("could not split link into name and target")
		}
	}

	response, err := json.Marshal(dto.FileHeader{
		Name:             name,
		EntryType:        entryType,
		LinkTarget:       linkTarget,
		Size:             size,
		ModificationTime: timestamp,
		Permissions:      permissions,
		Owner:            string(matches[headerLineGroupOwner]),
		Group:            string(matches[headerLineGroupGroup]),
	})
	if err != nil {
		return nil, fmt.Errorf("could not marshal file header: %w", err)
	}
	return response, nil
}
