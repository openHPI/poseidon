package nomad

import (
	"context"
	"fmt"
	"github.com/getsentry/sentry-go"
	"io"
	"regexp"
	"strconv"
	"time"
)

const (
	// timeDebugMessageFormat is the format of messages that will be converted to debug messages.
	timeDebugMessageFormat = "echo -ne \"\\x1EPoseidon %s $(date +%%s%%3N)\\x1E\""
	// Format Parameters: 1. Debug Comment, 2. command.
	timeDebugMessageFormatStart = timeDebugMessageFormat + "; %s"
	// Format Parameters: 1. command, 2. Debug Comment.
	timeDebugMessageFormatEnd = "%s && " + timeDebugMessageFormat

	timeDebugMessagePatternGroupText = 1
	timeDebugMessagePatternGroupTime = 2
)

var (
	timeDebugMessagePattern      = regexp.MustCompile(`\x1EPoseidon (?P<text>\w+) (?P<time>\d+)\x1E`)
	timeDebugMessagePatternStart = regexp.MustCompile(`\x1EPoseidon`)
)

// SentryDebugWriter is scanning the input for the debug message pattern.
// For matches, it creates a Sentry Span. Otherwise, the data will be forwarded to the Target.
// The passed context Ctx should contain the Sentry data.
type SentryDebugWriter struct {
	Target   io.Writer
	Ctx      context.Context
	lastSpan *sentry.Span
}

// Improve: Handling of a split debug messages (usually p is exactly one debug message, not less and not more).
func (s *SentryDebugWriter) Write(p []byte) (n int, err error) {
	if !timeDebugMessagePatternStart.Match(p) {
		count, err := s.Target.Write(p)
		if err != nil {
			err = fmt.Errorf("SentryDebugWriter Forwarded: %w", err)
		}
		return count, err
	}

	loc := timeDebugMessagePattern.FindIndex(p)
	if loc == nil {
		log.WithField("data", p).Warn("Exec debug message could not be read completely")
		return 0, nil
	}

	go s.handleTimeDebugMessage(p[loc[0]:loc[1]])

	debugMessageLength := loc[1] - loc[0]
	if debugMessageLength < len(p) {
		count, err := s.Write(append(p[0:loc[0]], p[loc[1]:]...))
		return debugMessageLength + count, err
	} else {
		return debugMessageLength, nil
	}
}

func (s *SentryDebugWriter) handleTimeDebugMessage(message []byte) {
	if s.lastSpan != nil {
		s.lastSpan.Finish()
	}

	matches := timeDebugMessagePattern.FindSubmatch(message)
	if matches == nil {
		log.WithField("msg", message).Error("Cannot parse passed time debug message")
		return
	}

	timestamp, err := strconv.ParseInt(string(matches[timeDebugMessagePatternGroupTime]), 10, 64)
	if err != nil {
		log.WithField("matches", matches).Warn("Could not parse Unix timestamp")
		return
	}
	s.lastSpan = sentry.StartSpan(s.Ctx, "nomad.execute.pipe")
	s.lastSpan.Description = string(matches[timeDebugMessagePatternGroupText])
	s.lastSpan.StartTime = time.UnixMilli(timestamp)
	s.lastSpan.Finish()

	s.lastSpan = sentry.StartSpan(s.Ctx, "nomad.execute.bash")
	s.lastSpan.Description = string(matches[timeDebugMessagePatternGroupText])
}
