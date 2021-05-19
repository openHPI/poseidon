package runner

const (
	defaultRunnerId      = "s0m3-r4nd0m-1d"
	anotherRunnerId      = "4n0th3r-runn3r-1d"
	defaultEnvironmentId = EnvironmentId(0)
	anotherEnvironmentId = EnvironmentId(42)
	defaultJobId         = "s0m3-j0b-1d"
	anotherJobId         = "4n0th3r-j0b-1d"
)

type DummyEntity struct{}

func (DummyEntity) Id() string {
	return ""
}
