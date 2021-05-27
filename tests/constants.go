package tests

import "errors"

const (
	NonExistingId                 = "n0n-3x1st1ng-1d"
	DefaultFileName               = "test.txt"
	DefaultFileContent            = "Hello, Codemoon!"
	DefaultDirectoryName          = "test/"
	FileNameWithAbsolutePath      = "/test.txt"
	DefaultEnvironmentIdAsInteger = 0
	DefaultEnvironmentIdAsString  = "0"
	AnotherEnvironmentIdAsInteger = 42
	AnotherEnvironmentIdAsString  = "42"
	DefaultJobId                  = "s0m3-j0b-1d"
	AnotherJobId                  = "4n0th3r-j0b-1d"
	DefaultRunnerId               = "s0m3-r4nd0m-1d"
	AnotherRunnerId               = "4n0th3r-runn3r-1d"
	DefaultExecutionId            = "s0m3-3x3cu710n-1d"
	DefaultMockId                 = "m0ck-1d"
)

var (
	DefaultError = errors.New("an error occurred")
)
