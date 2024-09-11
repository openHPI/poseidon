package nomad

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/nullio"
)

const (
	// unsetEnvironmentVariablesFormat prepends the call to unset the passed variables before the actual command.
	unsetEnvironmentVariablesFormat = "unset %s && %s"
	// unsetEnvironmentVariablesPrefix is the prefix of all environment variables that will be filtered.
	unsetEnvironmentVariablesPrefix = "NOMAD_"
	// unsetEnvironmentVariablesShell is the shell functionality to get all environment variables starting with the prefix.
	unsetEnvironmentVariablesShell = "${!" + unsetEnvironmentVariablesPrefix + "@}"

	// stderrFifoFormat represents the format we use for our stderr fifos. The %d should be unique for the execution
	// as otherwise multiple executions are not possible.
	// Example: "/tmp/stderr_1623330777825234133.fifo".
	stderrFifoFormat = "/tmp/stderr_%d.fifo"
	// stderrFifoCommandFormat, if executed, is supposed to create a fifo, read from it and remove it in the end.
	// Example: "mkfifo my.fifo && (cat my.fifo; rm my.fifo)".
	stderrFifoCommandFormat = "mkfifo %s && (cat %s; rm %s)"
	// stderrWrapperCommandFormat, if executed, is supposed to wait until a fifo exists (it sleeps 10ms to reduce load
	// cause by busy waiting on the system). Once the fifo exists, the given command is executed and its stderr
	// redirected to the fifo.
	// Example: "until [ -e my.fifo ]; do sleep 0.01; done; (echo \"my.fifo exists\") 2> my.fifo".
	stderrWrapperCommandFormat = "until [ -e %s ]; do sleep 0.01; done; (%s) 2> %s"

	// setUserBinaryPath is due to Poseidon requires the setuser script for Nomad environments.
	setUserBinaryPath = "/sbin/setuser"
	// setUserBinaryUser is the user that is used and required by Poseidon for Nomad environments.
	setUserBinaryUser = "user"
	// PrivilegedExecution is to indicate the privileged execution of the passed command.
	PrivilegedExecution = true
	// UnprivilegedExecution is to indicate the unprivileged execution of the passed command.
	UnprivilegedExecution = false
)

// ExecuteCommand executes the given command in the given job.
// If tty is true, Nomad would normally write stdout and stderr of the command
// both on the stdout stream. However, if the InteractiveStderr server config option is true,
// we make sure that stdout and stderr are split correctly.
func (a *APIClient) ExecuteCommand(ctx context.Context,
	jobID string, command string, tty bool, privilegedExecution bool,
	stdin io.Reader, stdout, stderr io.Writer,
) (int, error) {
	if tty && config.Config.Server.InteractiveStderr {
		return a.executeCommandInteractivelyWithStderr(ctx, jobID, command, privilegedExecution, stdin, stdout, stderr)
	}
	command = prepareCommandWithoutTTY(command, privilegedExecution)
	exitCode, err := a.apiQuerier.Execute(ctx, jobID, command, tty, stdin, stdout, stderr)
	if err != nil {
		return 1, fmt.Errorf("error executing command in job %s: %w", jobID, err)
	}
	return exitCode, nil
}

// executeCommandInteractivelyWithStderr executes the given command interactively and splits stdout
// and stderr correctly. Normally, using Nomad to execute a command with tty=true (in order to have
// an interactive connection and possibly a fully working shell), would result in stdout and stderr
// to be served both over stdout. This function circumvents this by creating a fifo for the stderr
// of the command and starting a second execution that reads the stderr from that fifo.
func (a *APIClient) executeCommandInteractivelyWithStderr(ctx context.Context, allocationID string,
	command string, privilegedExecution bool, stdin io.Reader, stdout, stderr io.Writer,
) (int, error) {
	// Use current nano time to make the stderr fifo kind of unique.
	currentNanoTime := time.Now().UnixNano()

	stderrExitChan := make(chan int)
	go func() {
		readingContext, cancel := context.WithCancel(ctx)
		defer cancel()

		// Catch stderr in separate execution.
		logging.StartSpan(ctx, "nomad.execute.stderr", "Execution for separate StdErr", func(ctx context.Context, _ *sentry.Span) {
			exit, err := a.Execute(ctx, allocationID, prepareCommandTTYStdErr(currentNanoTime, privilegedExecution), true,
				nullio.Reader{Ctx: readingContext}, stderr, io.Discard)
			if err != nil {
				log.WithContext(ctx).WithError(err).Warn("Stderr task finished with error")
			}
			stderrExitChan <- exit
		})
	}()

	command = prepareCommandTTY(command, currentNanoTime, privilegedExecution)
	var exit int
	var err error
	logging.StartSpan(ctx, "nomad.execute.tty", "Interactive Execution", func(ctx context.Context, _ *sentry.Span) {
		exit, err = a.Execute(ctx, allocationID, command, true, stdin, stdout, io.Discard)
	})

	// Wait until the stderr catch command finished to make sure we receive all output.
	<-stderrExitChan
	return exit, err
}

func prepareCommandWithoutTTY(command string, privilegedExecution bool) string {
	const commandFieldAfterEnv = 4 // instead of "env CODEOCEAN=true /bin/bash -c sleep infinity" just "sleep infinity".
	command = setInnerDebugMessages(command, commandFieldAfterEnv, -1)

	command = setUserCommand(command, privilegedExecution)
	command = unsetEnvironmentVariables(command)
	return command
}

func prepareCommandTTY(command string, currentNanoTime int64, privilegedExecution bool) string {
	const commandFieldAfterSettingEnvVariables = 4
	command = setInnerDebugMessages(command, commandFieldAfterSettingEnvVariables, -1)

	// Take the command to be executed and wrap it to redirect stderr.
	stderrFifoPath := stderrFifo(currentNanoTime)
	command = fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoPath, command, stderrFifoPath)
	command = injectStartDebugMessage(command, 0, 1)

	command = setUserCommand(command, privilegedExecution)
	command = unsetEnvironmentVariables(command)
	return command
}

func prepareCommandTTYStdErr(currentNanoTime int64, privilegedExecution bool) string {
	stderrFifoPath := stderrFifo(currentNanoTime)
	command := fmt.Sprintf(stderrFifoCommandFormat, stderrFifoPath, stderrFifoPath, stderrFifoPath)
	command = setInnerDebugMessages(command, 0, 1)
	command = setUserCommand(command, privilegedExecution)
	return command
}

func stderrFifo(id int64) string {
	return fmt.Sprintf(stderrFifoFormat, id)
}

func unsetEnvironmentVariables(command string) string {
	command = dto.WrapBashCommand(command)
	command = fmt.Sprintf(unsetEnvironmentVariablesFormat, unsetEnvironmentVariablesShell, command)

	// Debug Message
	const commandFieldBeforeBash = 2 // e.g. instead of "unset ${!NOMAD_@} && /bin/bash -c [...]" just "unset ${!NOMAD_@}".
	command = injectStartDebugMessage(command, 0, commandFieldBeforeBash)
	return command
}

// setUserCommand prefixes the passed command with the setUser command.
func setUserCommand(command string, privilegedExecution bool) string {
	// Wrap the inner command first so that the setUserBinary applies to the whole inner command.
	command = dto.WrapBashCommand(command)

	if !privilegedExecution {
		command = fmt.Sprintf("%s %s %s", setUserBinaryPath, setUserBinaryUser, command)
	}

	// Debug Message
	const commandFieldBeforeBash = 2 // e.g. instead of "/sbin/setuser user /bin/bash -c [...]" just "/sbin/setuser user".
	command = injectStartDebugMessage(command, 0, commandFieldBeforeBash)
	return command
}

func injectStartDebugMessage(command string, start uint, end int) string {
	commandFields := strings.Fields(command)
	if start < uint(len(commandFields)) {
		commandFields = commandFields[start:]

		if (end < 0 && start > uint(math.MaxInt32)) || (end > 0 && start > uint(math.MaxInt32)-uint(end)) {
			log.WithField("start", start).Error("passed start too big")
		}
		end -= int(start)
	}
	if end >= 0 && end < len(commandFields) {
		commandFields = commandFields[:end]
	}

	description := strings.Join(commandFields, " ")
	if strings.HasPrefix(description, "\"") && strings.HasSuffix(description, "\"") {
		description = description[1 : len(description)-1]
	}

	description = dto.BashEscapeCommand(description)
	description = description[1 : len(description)-1] // The most outer quotes are not escaped!
	return fmt.Sprintf(timeDebugMessageFormatStart, description, command)
}

// setInnerDebugMessages injects debug commands into the bash command.
// The debug messages are parsed by the SentryDebugWriter.
func setInnerDebugMessages(command string, descriptionStart uint, descriptionEnd int) (result string) {
	result = injectStartDebugMessage(command, descriptionStart, descriptionEnd)

	result = strings.TrimSuffix(result, ";")
	return fmt.Sprintf(timeDebugMessageFormatEnd, result, "exit $ec")
}
