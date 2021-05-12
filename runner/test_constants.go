package runner

const RunnerId = "s0m3-r4nd0m-1d"

func CreateTestRunner() Runner {
	return NewRunner(RunnerId)
}
