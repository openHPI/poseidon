package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHelloCodeMoon(t *testing.T) {
	assert.Equal(t, "Hello, CodeMoon!", HelloCodeMoon())
}
