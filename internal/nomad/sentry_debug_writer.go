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
	// Format Parameters: 1. Debug Comment, 2. command.
	timeDebugMessageFormatStart = `echo -ne "\x1EPoseidon %s $(date +%%s%%3N)\x1E"; %s`
	// Format Parameters: 1. command, 2. Debug Comment.
	timeDebugMessageFormatEnd = `%s; bash -c "ec=$?; echo -ne \"\\x1EPoseidon %s \$(date +%%s%%3N)\\x1E\" && exit \$ec"`
)

var (
	timeDebugMessagePattern = regexp.
				MustCompile(`(?P<before>.*)\x1EPoseidon (?P<text>.+) (?P<time>\d{13})\x1E(?P<after>.*)`)
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

func NewSentryDebugWriter(target io.Writer, ctx context.Context) *SentryDebugWriter {
	span := sentry.StartSpan(ctx, "nomad.execute.connect")
	return &SentryDebugWriter{
		Target:   target,
		Ctx:      ctx,
		lastSpan: span,
	}
}

// Improve: Handling of a split debug messages (usually, p is exactly one debug message, not less and not more).
func (s *SentryDebugWriter) Write(p []byte) (n int, err error) {
	if !timeDebugMessagePatternStart.Match(p) {
		count, err := s.Target.Write(p)
		if err != nil {
			err = fmt.Errorf("SentryDebugWriter Forwarded Error: %w", err)
		}
		return count, err
	}

	match := matchAndMapTimeDebugMessage(p)
	if match == nil {
		log.WithField("data", p).Warn("Exec debug message could not be read completely")
		return 0, nil
	}

	go s.handleTimeDebugMessage(match)

	lengthRemainingData := len(match["before"]) + len(match["after"])
	if lengthRemainingData > 0 {
		count, err := s.Write(append(match["before"], match["after"]...))
		return len(p) - (lengthRemainingData - count), err
	} else {
		return len(p), nil
	}
}

func (s *SentryDebugWriter) Close(exitCode int) {
	if s.lastSpan != nil {
		s.lastSpan.Op = "nomad.execute.disconnect"
		s.lastSpan.SetTag("exit_code", strconv.Itoa(exitCode))
		s.lastSpan.Finish()
	}
}

// handleTimeDebugMessage transforms one time debug message into a Sentry span.
// It requires match to contain the keys `time` and `text`.
func (s *SentryDebugWriter) handleTimeDebugMessage(match map[string][]byte) {
	timestamp, err := strconv.ParseInt(string(match["time"]), 10, 64)
	if err != nil {
		log.WithField("match", match).Warn("Could not parse Unix timestamp")
		return
	}

	if s.lastSpan != nil {
		s.lastSpan.EndTime = time.UnixMilli(timestamp)
		s.lastSpan.SetData("latency", time.Since(time.UnixMilli(timestamp)).String())
		s.lastSpan.Finish()
	}

	s.lastSpan = sentry.StartSpan(s.Ctx, "nomad.execute.bash")
	s.lastSpan.Description = string(match["text"])
	s.lastSpan.StartTime = time.UnixMilli(timestamp)
}

func matchAndMapTimeDebugMessage(p []byte) map[string][]byte {
	match := timeDebugMessagePattern.FindSubmatch(p)
	if match == nil {
		return nil
	}

	labelMap := make(map[string][]byte)
	for i, name := range timeDebugMessagePattern.SubexpNames() {
		if i != 0 && name != "" {
			labelMap[name] = match[i]
		}
	}
	return labelMap
}
