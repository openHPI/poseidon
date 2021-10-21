package runner

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
)

const (
	defaultEnvironmentID     = dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger)
	anotherEnvironmentID     = dto.EnvironmentID(tests.AnotherEnvironmentIDAsInteger)
	defaultInactivityTimeout = 0
)
