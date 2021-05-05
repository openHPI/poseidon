package environment

import "gitlab.hpi.de/codeocean/codemoon/poseidon/runner"

const RunnerId = "s0m3-r4nd0m-1d"

func CreateTestRunner() runner.Runner {
	return runner.NewExerciseRunner(RunnerId)
}
