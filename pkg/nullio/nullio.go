package nullio

import (
	"context"
	"fmt"
	"io"
)

// Reader is a struct that implements the io.Reader interface.
// Read does not return when called until the context is done. It is used to avoid reading anything and returning io.EOF
// before the context has finished.
// For example the reader is used by the execution that fetches the stderr stream from Nomad. We do not have a stdin
// that we want to send to Nomad. But we still have to pass Nomad a reader.
// Normally readers send an io.EOF as soon as they have nothing more to read. But we want to avoid this, because in that
// case Nomad will abort (not the execution but) the transmission.
// Why should the reader not just always return 0, nil? Because Nomad reads in an endless loop and thus a busy waiting
// is avoided.
type Reader struct {
	//nolint:containedctx // See #630.
	Ctx context.Context
}

func (r Reader) Read(_ []byte) (int, error) {
	if r.Ctx == nil || r.Ctx.Err() != nil {
		return 0, io.EOF
	}

	<-r.Ctx.Done()
	return 0, io.EOF
}

// ReadWriter implements io.ReadWriter. It does not return from Read and discards everything on Write.
type ReadWriter struct {
	Reader
}

func (rw *ReadWriter) Write(p []byte) (int, error) {
	n, err := io.Discard.Write(p)
	if err != nil {
		return n, fmt.Errorf("error writing to io.Discard: %w", err)
	} else {
		return n, nil
	}
}
