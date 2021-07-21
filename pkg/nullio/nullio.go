package nullio

import (
	"fmt"
	"io"
)

// Reader is a struct that implements the io.Reader interface. Read does not return when called.
type Reader struct{}

func (r Reader) Read(_ []byte) (int, error) {
	// An empty select blocks forever.
	select {}
}

// ReadWriter implements io.ReadWriter and does nothing on Read an Write.
type ReadWriter struct {
	Reader
}

func (nrw *ReadWriter) Write(p []byte) (int, error) {
	n, err := io.Discard.Write(p)
	return n, fmt.Errorf("error writing to io.Discard: %w", err)
}
