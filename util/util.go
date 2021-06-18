package util

// NullReader is a struct that implements the io.Reader interface and returns nothing when reading
// from it.
type NullReader struct{}

func (r NullReader) Read(_ []byte) (int, error) {
	// An empty select blocks forever.
	select {}
}
