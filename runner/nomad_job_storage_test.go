package runner

import (
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"testing"
)

func TestJobStoreTestSuite(t *testing.T) {
	suite.Run(t, new(JobStoreTestSuite))
}

type JobStoreTestSuite struct {
	suite.Suite
	jobStorage *localNomadJobStorage
	job        *NomadJob
}

func (suite *JobStoreTestSuite) SetupTest() {
	suite.jobStorage = NewLocalNomadJobStorage()
	suite.job = &NomadJob{environmentId: defaultEnvironmentId, jobId: tests.DefaultJobId}
}

func (suite *JobStoreTestSuite) TestAddedJobCanBeRetrieved() {
	suite.jobStorage.Add(suite.job)
	retrievedJob, ok := suite.jobStorage.Get(suite.job.Id())
	suite.True(ok, "A saved runner should be retrievable")
	suite.Equal(suite.job, retrievedJob)
}

func (suite *JobStoreTestSuite) TestJobWithSameIdOverwritesOldOne() {
	otherJobWithSameId := &NomadJob{environmentId: defaultEnvironmentId}
	// assure runner is actually different
	otherJobWithSameId.jobId = tests.AnotherJobId
	suite.NotEqual(suite.job, otherJobWithSameId)

	suite.jobStorage.Add(suite.job)
	suite.jobStorage.Add(otherJobWithSameId)
	retrievedJob, _ := suite.jobStorage.Get(suite.job.Id())
	suite.NotEqual(suite.job, retrievedJob)
	suite.Equal(otherJobWithSameId, retrievedJob)
}

func (suite *JobStoreTestSuite) TestDeletedJobIsNotAccessible() {
	suite.jobStorage.Add(suite.job)
	suite.jobStorage.Delete(suite.job.Id())
	retrievedRunner, ok := suite.jobStorage.Get(suite.job.Id())
	suite.Nil(retrievedRunner)
	suite.False(ok, "A deleted runner should not be accessible")
}

func (suite *JobStoreTestSuite) TestLenOfEmptyPoolIsZero() {
	suite.Equal(0, suite.jobStorage.Length())
}

func (suite *JobStoreTestSuite) TestLenChangesOnStoreContentChange() {
	suite.Run("len increases when job is added", func() {
		suite.jobStorage.Add(suite.job)
		suite.Equal(1, suite.jobStorage.Length())
	})

	suite.Run("len does not increase when job with same id is added", func() {
		suite.jobStorage.Add(suite.job)
		suite.Equal(1, suite.jobStorage.Length())
	})

	suite.Run("len increases again when different job is added", func() {
		anotherJob := &NomadJob{environmentId: anotherEnvironmentId}
		suite.jobStorage.Add(anotherJob)
		suite.Equal(2, suite.jobStorage.Length())
	})

	suite.Run("len decreases when job is deleted", func() {
		suite.jobStorage.Delete(suite.job.Id())
		suite.Equal(1, suite.jobStorage.Length())
	})
}
