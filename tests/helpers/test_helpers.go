// Package helpers contains functions that help executing tests.
// The helper functions generally look from the client side - a Poseidon user.
package helpers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BuildURL joins multiple route paths.
func BuildURL(parts ...string) string {
	url := config.Config.PoseidonAPIURL().String()
	for _, part := range parts {
		if !strings.HasPrefix(part, "/") {
			url += "/"
		}
		url += part
	}
	return url
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
	return stdout, stderr, errors
}

// WebSocketControlMessages extracts all meta (and exit) messages from the passed messages.
func WebSocketControlMessages(messages []*dto.WebSocketMessage) (controls []*dto.WebSocketMessage) {
	for _, msg := range messages {
		switch msg.Type {
		case dto.WebSocketMetaStart, dto.WebSocketMetaTimeout, dto.WebSocketExit:
			controls = append(controls, msg)
		}
	}
	return controls
}

// ReceiveAllWebSocketMessages pulls all messages from the websocket connection without sending anything.
// This function does not return unless the server closes the connection or a readDeadline is set
// in the WebSocket connection.
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
// This function does not return unless the server sends a message, closes the connection or a readDeadline
// is set in the WebSocket connection.
func ReceiveNextWebSocketMessage(connection *websocket.Conn) (*dto.WebSocketMessage, error) {
	_, reader, err := connection.NextReader()
	if err != nil {
		//nolint:wrapcheck // we could either wrap here and do complicated things with errors.As or just not wrap
		// the error in this test function and allow tests to use equal
		return nil, err
	}
	message := new(dto.WebSocketMessage)
	err = json.NewDecoder(reader).Decode(message)
	if err != nil {
		return nil, fmt.Errorf("error decoding WebSocket message: %w", err)
	}
	return message, nil
}

// StartTLSServer runs a httptest.Server with the passed mux.Router and TLS enabled.
func StartTLSServer(t *testing.T, router *mux.Router) (*httptest.Server, error) {
	t.Helper()
	dir := t.TempDir()
	keyOut := filepath.Join(dir, "poseidon-test.key")
	certOut := filepath.Join(dir, "poseidon-test.crt")

	err := exec.Command("openssl", "req", "-x509", "-nodes", "-newkey", "rsa:2048",
		"-keyout", keyOut, "-out", certOut, "-days", "1",
		"-subj", "/CN=Poseidon test", "-addext", "subjectAltName=IP:127.0.0.1,DNS:localhost").Run()
	if err != nil {
		return nil, fmt.Errorf("error creating self-signed cert: %w", err)
	}
	cert, err := tls.LoadX509KeyPair(certOut, keyOut)
	if err != nil {
		return nil, fmt.Errorf("error loading x509 key pair: %w", err)
	}

	server := httptest.NewUnstartedServer(router)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	return server, nil
}

// HTTPDelete sends a Delete Http Request with body to the passed url.
func HTTPDelete(url string, body io.Reader) (response *http.Response, err error) {
	//nolint:noctx // we don't need a http.NewRequestWithContext in our tests
	req, err := http.NewRequest(http.MethodDelete, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	client := &http.Client{}
	response, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	return response, nil
}

// HTTPPatch sends a Patch Http Request with body to the passed url.
func HTTPPatch(url, contentType string, body io.Reader) (response *http.Response, err error) {
	//nolint:noctx // we don't need a http.NewRequestWithContext in our tests
	req, err := http.NewRequest(http.MethodPatch, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	return resp, nil
}

func HTTPPut(url string, body io.Reader) (response *http.Response, err error) {
	//nolint:noctx // we don't need a http.NewRequestWithContext in our tests
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	return resp, nil
}

func HTTPPutJSON(url string, body interface{}) (response *http.Response, err error) {
	requestByteString, err := json.Marshal(body)
	if err != nil {
		return
	}
	reader := bytes.NewReader(requestByteString)
	return HTTPPut(url, reader)
}

func CreateTemplateJob() (base, job *nomadApi.Job) {
	base = nomadApi.NewBatchJob(tests.DefaultJobID, tests.DefaultJobID, "region-name", 100)
	job = nomadApi.NewBatchJob(tests.DefaultJobID, tests.DefaultJobID, "region-name", 100)
	configTaskGroup := nomadApi.NewTaskGroup("config", 0)
	configTaskGroup.Meta = make(map[string]string)
	configTaskGroup.Meta["prewarmingPoolSize"] = "0"
	configTask := nomadApi.NewTask("config", "exec")
	configTask.Config = map[string]interface{}{"command": "true"}
	configTaskGroup.AddTask(configTask)

	defaultTaskGroup := nomadApi.NewTaskGroup("default-group", 1)
	defaultTaskGroup.Networks = []*nomadApi.NetworkResource{}
	defaultTask := nomadApi.NewTask("default-task", "docker")
	defaultTask.Config = map[string]interface{}{
		"image":        "python:latest",
		"command":      "sleep",
		"args":         []string{"infinity"},
		"network_mode": "none",
	}
	defaultTask.Resources = nomadApi.DefaultResources()
	defaultTaskGroup.AddTask(defaultTask)

	job.TaskGroups = []*nomadApi.TaskGroup{defaultTaskGroup, configTaskGroup}
	return base, job
}
