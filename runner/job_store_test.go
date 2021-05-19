package runner

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestJobStoreTestSuite(t *testing.T) {
	suite.Run(t, new(JobStoreTestSuite))
}

type JobStoreTestSuite struct {
	suite.Suite
	jobStore *nomadJobStore
	job      *NomadJob
}

func (suite *JobStoreTestSuite) SetupTest() {
	suite.jobStore = NewNomadJobStore()
	suite.job = &NomadJob{environmentId: defaultEnvironmentId, jobId: defaultJobId}
}

func (suite *JobStoreTestSuite) TestAddInvalidEntityTypeThrowsFatal() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	// don't terminate program on fatal log entry
	logger.ExitFunc = func(int) {}
	log = logger.WithField("pkg", "environment")

	dummyEntity := DummyEntity{}
	suite.jobStore.Add(dummyEntity)
	suite.Equal(logrus.FatalLevel, hook.LastEntry().Level)
	suite.Equal(dummyEntity, hook.LastEntry().Data["entity"])
}

func (suite *JobStoreTestSuite) TestAddValidEntityDoesNotThrowFatal() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "environment")

	suite.jobStore.Add(suite.job)
	// currently, the Add method does not log anything else. adjust if necessary
	suite.Nil(hook.LastEntry())
}

func (suite *JobStoreTestSuite) TestAddedJobCanBeRetrieved() {
	suite.jobStore.Add(suite.job)
	retrievedJob, ok := suite.jobStore.Get(suite.job.Id())
	suite.True(ok, "A saved runner should be retrievable")
	suite.Equal(suite.job, retrievedJob)
}

func (suite *JobStoreTestSuite) TestJobWithSameIdOverwritesOldOne() {
	otherJobWithSameId := &NomadJob{environmentId: defaultEnvironmentId}
	// assure runner is actually different
	otherJobWithSameId.jobId = anotherJobId
	suite.NotEqual(suite.job, otherJobWithSameId)

	suite.jobStore.Add(suite.job)
	suite.jobStore.Add(otherJobWithSameId)
	retrievedJob, _ := suite.jobStore.Get(suite.job.Id())
	suite.NotEqual(suite.job, retrievedJob)
	suite.Equal(otherJobWithSameId, retrievedJob)
}

func (suite *JobStoreTestSuite) TestDeletedJobIsNotAccessible() {
	suite.jobStore.Add(suite.job)
	suite.jobStore.Delete(suite.job.Id())
	retrievedRunner, ok := suite.jobStore.Get(suite.job.Id())
	suite.Nil(retrievedRunner)
	suite.False(ok, "A deleted runner should not be accessible")
}

func (suite *JobStoreTestSuite) TestLenOfEmptyPoolIsZero() {
	suite.Equal(0, suite.jobStore.Len())
}

func (suite *JobStoreTestSuite) TestLenChangesOnStoreContentChange() {
	suite.Run("len increases when job is added", func() {
		suite.jobStore.Add(suite.job)
		suite.Equal(1, suite.jobStore.Len())
	})

	suite.Run("len does not increase when job with same id is added", func() {
		suite.jobStore.Add(suite.job)
		suite.Equal(1, suite.jobStore.Len())
	})

	suite.Run("len increases again when different job is added", func() {
		anotherJob := &NomadJob{environmentId: anotherEnvironmentId}
		suite.jobStore.Add(anotherJob)
		suite.Equal(2, suite.jobStore.Len())
	})

	suite.Run("len decreases when job is deleted", func() {
		suite.jobStore.Delete(suite.job.Id())
		suite.Equal(1, suite.jobStore.Len())
	})
}
