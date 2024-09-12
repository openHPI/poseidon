package nomad

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/pkg/logging"
)

var (
	// timeDebugMessageFormat is the format of messages that will be converted to debug messages.
	timeDebugMessageFormat = `echo -ne "\x1EPoseidon %s $(date +%%s%%3N)\x1E"`
	// Format Parameters: 1. Debug Comment, 2. command.
	timeDebugMessageFormatStart = timeDebugMessageFormat + `; %s`
	// Format Parameters: 1. command, 2. Debug Comment.
	timeDebugMessageFormatEnd = `%s; ec=$?; ` + timeDebugMessageFormat + ` && exit $ec`

	timeDebugMessagePattern = regexp.MustCompile(
		`(?P<before>[\S\s]*?)\x1EPoseidon (?P<text>[^\x1E]+?) (?P<time>\d{13})\x1E(?P<after>[\S\s]*)`)
	timeDebugMessagePatternStart    = regexp.MustCompile(`\x1EPoseidon`)
	timeDebugMessagePatternStartEnd = regexp.MustCompile(`\x1EPoseidon.*\x1E`)
	timeDebugFallbackDescription    = "<empty>"
)

// SentryDebugWriter is scanning the input for the debug message pattern.
// For matches, it creates a Sentry Span. Otherwise, the data will be forwarded to the Target.
// The passed context Ctx should contain the Sentry data.
type SentryDebugWriter struct {
	Target io.Writer
	//nolint:containedctx // See #630.
	Ctx       context.Context
	lastSpan  *sentry.Span
	remaining *bytes.Buffer
}

func NewSentryDebugWriter(ctx context.Context, target io.Writer) *SentryDebugWriter {
	ctx = logging.CloneSentryHub(ctx)
	span := sentry.StartSpan(ctx, "nomad.execute.connect")
	span.Description = "/bin/bash -c"
	return &SentryDebugWriter{
		Target:    target,
		Ctx:       ctx,
		lastSpan:  span,
		remaining: &bytes.Buffer{},
	}
}

// Improve: Handling of a split debug messages (usually, p is exactly one debug message, not less and not more).
func (s *SentryDebugWriter) Write(debugData []byte) (n int, err error) {
	if s.Ctx.Err() != nil {
		return 0, fmt.Errorf("SentryDebugWriter context error: %w", s.Ctx.Err())
	}
	// Peaking if the target is able to write.
	// If not we should not process the data (see #325).
	if _, err = s.Target.Write([]byte{}); err != nil {
		return 0, fmt.Errorf("SentryDebugWriter cannot write to target: %w", err)
	}
	log.WithContext(s.lastSpan.Context()).WithField("data", fmt.Sprintf("%q", debugData)).Trace("Received data from Nomad container")

	if s.remaining.Len() > 0 {
		n -= s.remaining.Len()
		joinedDebugData := make([]byte, 0, s.remaining.Len()+len(debugData))
		joinedDebugData = append(joinedDebugData, s.remaining.Bytes()...)
		joinedDebugData = append(joinedDebugData, debugData...)
		s.remaining.Reset()
		debugData = joinedDebugData
	}

	if !timeDebugMessagePatternStart.Match(debugData) {
		count, err := s.Target.Write(debugData)
		if err != nil {
			err = fmt.Errorf("SentryDebugWriter Forwarded Error: %w", err)
		}
		return count, err
	}

	if !timeDebugMessagePatternStartEnd.Match(debugData) {
		count, err := s.remaining.Write(debugData)
		if err != nil {
			err = fmt.Errorf("SentryDebugWriter failed to buffer data: %w", err)
		}
		return count, err
	}

	match := matchAndMapTimeDebugMessage(debugData)
	if match == nil {
		log.WithContext(s.lastSpan.Context()).WithField("data", fmt.Sprintf("%q", debugData)).Warn("Exec debug message could not be read completely")
		return 0, nil
	}

	if len(match["before"]) > 0 {
		var count int
		count, err = s.Write(match["before"])
		n += count
	}

	s.handleTimeDebugMessage(match)
	n += len(debugData) - len(match["before"]) - len(match["after"])

	if len(match["after"]) > 0 {
		var count int
		count, err = s.Write(match["after"])
		n += count
	}

	return n, err
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
		log.WithContext(s.lastSpan.Context()).WithField("match", match).Warn("Could not parse Unix timestamp")
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
