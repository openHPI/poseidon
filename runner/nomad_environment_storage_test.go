package runner

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestJobStoreTestSuite(t *testing.T) {
	suite.Run(t, new(JobStoreTestSuite))
}

type JobStoreTestSuite struct {
	suite.Suite
	jobStorage *localNomadJobStorage
	job        *NomadEnvironment
}

func (suite *JobStoreTestSuite) SetupTest() {
	suite.jobStorage = NewLocalNomadJobStorage()
	suite.job = &NomadEnvironment{environmentID: defaultEnvironmentId}
}

func (s *JobStoreTestSuite) TestAddedJobCanBeRetrieved() {
	s.jobStorage.Add(s.job)
	retrievedJob, ok := s.jobStorage.Get(s.job.ID())
	s.True(ok, "A saved runner should be retrievable")
	s.Equal(s.job, retrievedJob)
}

func (suite *JobStoreTestSuite) TestJobWithSameIdOverwritesOldOne() {
	otherJobWithSameID := &NomadEnvironment{environmentID: defaultEnvironmentId}
	otherJobWithSameID.templateJob = &nomadApi.Job{}
	suite.NotEqual(suite.job, otherJobWithSameID)

	s.jobStorage.Add(s.job)
	s.jobStorage.Add(otherJobWithSameID)
	retrievedJob, _ := s.jobStorage.Get(s.job.ID())
	s.NotEqual(s.job, retrievedJob)
	s.Equal(otherJobWithSameID, retrievedJob)
}

func (s *JobStoreTestSuite) TestDeletedJobIsNotAccessible() {
	s.jobStorage.Add(s.job)
	s.jobStorage.Delete(s.job.ID())
	retrievedRunner, ok := s.jobStorage.Get(s.job.ID())
	s.Nil(retrievedRunner)
	s.False(ok, "A deleted runner should not be accessible")
}

func (s *JobStoreTestSuite) TestLenOfEmptyPoolIsZero() {
	s.Equal(0, s.jobStorage.Length())
}

func (s *JobStoreTestSuite) TestLenChangesOnStoreContentChange() {
	s.Run("len increases when job is added", func() {
		s.jobStorage.Add(s.job)
		s.Equal(1, s.jobStorage.Length())
	})

	s.Run("len does not increase when job with same id is added", func() {
		s.jobStorage.Add(s.job)
		s.Equal(1, s.jobStorage.Length())
	})

	s.Run("len increases again when different job is added", func() {
		anotherJob := &NomadEnvironment{environmentID: anotherEnvironmentID}
		s.jobStorage.Add(anotherJob)
		s.Equal(2, s.jobStorage.Length())
	})

	s.Run("len decreases when job is deleted", func() {
		s.jobStorage.Delete(s.job.ID())
		s.Equal(1, s.jobStorage.Length())
	})
}
