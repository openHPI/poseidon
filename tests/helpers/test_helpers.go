// Package helpers contains functions that help executing tests.
// The helper functions generally look from the client side - a Poseidon user.
package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/mock"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BuildURL joins multiple route paths.
func BuildURL(parts ...string) (url string) {
	url = config.Config.PoseidonAPIURL().String()
	for _, part := range parts {
		if !strings.HasPrefix(part, "/") {
			url += "/"
		}
		url += part
	}
	return
}

// WebSocketOutputMessages extracts all stdout, stderr and error messages from the passed messages.
// It is useful since Nomad splits the command output nondeterministic.
func WebSocketOutputMessages(messages []*dto.WebSocketMessage) (stdout, stderr string, errors []string) {
	for _, msg := range messages {
		switch msg.Type {
		case dto.WebSocketOutputStdout:
			stdout += msg.Data
		case dto.WebSocketOutputStderr:
			stderr += msg.Data
		case dto.WebSocketOutputError:
			errors = append(errors, msg.Data)
		}
	}
	return
}

// WebSocketControlMessages extracts all meta (and exit) messages from the passed messages.
func WebSocketControlMessages(messages []*dto.WebSocketMessage) (controls []*dto.WebSocketMessage) {
	for _, msg := range messages {
		switch msg.Type {
		case dto.WebSocketMetaStart, dto.WebSocketMetaTimeout, dto.WebSocketExit:
			controls = append(controls, msg)
		}
	}
	return
}

// ReceiveAllWebSocketMessages pulls all messages from the websocket connection without sending anything.
// This function does not return unless the server closes the connection or a readDeadline is set in the WebSocket connection.
func ReceiveAllWebSocketMessages(connection *websocket.Conn) (messages []*dto.WebSocketMessage, err error) {
	for {
		var message *dto.WebSocketMessage
		message, err = ReceiveNextWebSocketMessage(connection)
		if err != nil {
			return
		}
		messages = append(messages, message)
	}
}

// ReceiveNextWebSocketMessage pulls the next message from the websocket connection.
// This function does not return unless the server sends a message, closes the connection or a readDeadline is set in the WebSocket connection.
func ReceiveNextWebSocketMessage(connection *websocket.Conn) (*dto.WebSocketMessage, error) {
	_, reader, err := connection.NextReader()
	if err != nil {
		return nil, err
	}
	message := new(dto.WebSocketMessage)
	err = json.NewDecoder(reader).Decode(message)
	if err != nil {
		return nil, err
	}
	return message, nil
}

// StartTLSServer runs a httptest.Server with the passed mux.Router and TLS enabled.
func StartTLSServer(t *testing.T, router *mux.Router) (server *httptest.Server, err error) {
	dir := t.TempDir()
	keyOut := filepath.Join(dir, "poseidon-test.key")
	certOut := filepath.Join(dir, "poseidon-test.crt")

	err = exec.Command("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048",
		"-keyout", keyOut, "-out", certOut, "-days", "1",
		"-subj", "/CN=Poseidon test", "-addext", "subjectAltName=IP:127.0.0.1,DNS:localhost").Run()
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certOut, keyOut)
	if err != nil {
		return nil, err
	}

	server = httptest.NewUnstartedServer(router)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	return
}

// MockApiExecute mocks the ExecuteCommand method of an ExecutorApi to call the given method run when the command
// corresponding to the given ExecutionRequest is called.
func MockApiExecute(api *nomad.ExecutorApiMock, request *dto.ExecutionRequest,
	run func(runnerId string, ctx context.Context, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) (int, error)) {
	call := api.On("ExecuteCommand",
		mock.AnythingOfType("string"),
		mock.Anything,
		request.FullCommand(),
		mock.AnythingOfType("bool"),
		mock.Anything,
		mock.Anything,
		mock.Anything)
	call.Run(func(args mock.Arguments) {
		exit, err := run(args.Get(0).(string),
			args.Get(1).(context.Context),
			args.Get(2).([]string),
			args.Get(3).(bool),
			args.Get(4).(io.Reader),
			args.Get(5).(io.Writer),
			args.Get(6).(io.Writer))
		call.ReturnArguments = mock.Arguments{exit, err}
	})
}

// HttpDelete sends a Delete Http Request with body to the passed url.
func HttpDelete(url string, body io.Reader) (response *http.Response, err error) {
	req, _ := http.NewRequest(http.MethodDelete, url, body)
	client := &http.Client{}
	return client.Do(req)
}

// HttpPatch sends a Patch Http Request with body to the passed url.
func HttpPatch(url string, contentType string, body io.Reader) (response *http.Response, err error) {
	req, _ := http.NewRequest(http.MethodPatch, url, body)
	req.Header.Set("Content-Type", contentType)
	client := &http.Client{}
	return client.Do(req)
}

func HttpPut(url string, body io.Reader) (response *http.Response, err error) {
	req, _ := http.NewRequest(http.MethodPut, url, body)
	client := &http.Client{}
	return client.Do(req)
}

func HttpPutJSON(url string, body interface{}) (response *http.Response, err error) {
	requestByteString, err := json.Marshal(body)
	if err != nil {
		return
	}
	reader := bytes.NewReader(requestByteString)
	return HttpPut(url, reader)
}
