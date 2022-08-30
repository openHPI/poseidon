package nullio

import (
	"bytes"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestLs2JsonTestSuite(t *testing.T) {
	suite.Run(t, new(Ls2JsonTestSuite))
}

type Ls2JsonTestSuite struct {
	suite.Suite
	buf    *bytes.Buffer
	writer *Ls2JsonWriter
}

func (s *Ls2JsonTestSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
	s.writer = &Ls2JsonWriter{Target: s.buf}
}

func (s *Ls2JsonTestSuite) TestLs2JsonWriter_WriteCreationAndClose() {
	count, err := s.writer.Write([]byte(""))
	s.Zero(count)
	s.NoError(err)

	s.Equal("{\"files\": [", s.buf.String())

	s.writer.Close()
	s.Equal("{\"files\": []}", s.buf.String())
}

func (s *Ls2JsonTestSuite) TestLs2JsonWriter_WriteFile() {
	input := "total 0\n-rw-rw-r-- 1 kali kali 0 1660763446 flag\n"
	count, err := s.writer.Write([]byte(input))
	s.Equal(len(input), count)
	s.NoError(err)
	s.writer.Close()

	s.Equal("{\"files\": [{\"name\":\"flag\",\"entryType\":\"-\",\"size\":0,\"modificationTime\":1660763446"+
		",\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"}]}",
		s.buf.String())
}

func (s *Ls2JsonTestSuite) TestLs2JsonWriter_WriteRecursive() {
	input := ".:\ntotal 4\ndrwxrwxr-x 2 kali kali 4096 1660764411 dir\n" +
		"-rw-rw-r-- 1 kali kali    0 1660763446 flag\n" +
		"\n./dir:\ntotal 4\n-rw-rw-r-- 1 kali kali 3 1660764366 another.txt\n"
	count, err := s.writer.Write([]byte(input))
	s.Equal(len(input), count)
	s.NoError(err)
	s.writer.Close()

	s.Equal("{\"files\": ["+
		"{\"name\":\"./dir\",\"entryType\":\"d\",\"size\":4096,\"modificationTime\":1660764411,"+
		"\"permissions\":\"rwxrwxr-x\",\"owner\":\"kali\",\"group\":\"kali\"},"+
		"{\"name\":\"./flag\",\"entryType\":\"-\",\"size\":0,\"modificationTime\":1660763446,"+
		"\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"},"+
		"{\"name\":\"./dir/another.txt\",\"entryType\":\"-\",\"size\":3,\"modificationTime\":1660764366,"+
		"\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"}"+
		"]}",
		s.buf.String())
}

func (s *Ls2JsonTestSuite) TestLs2JsonWriter_WriteRemaining() {
	input1 := "total 4\n-rw-rw-r-- 1 kali kali 3 1660764366 an.txt\n-rw-rw-r-- 1 kal"
	_, err := s.writer.Write([]byte(input1))
	s.NoError(err)
	s.Equal("{\"files\": [{\"name\":\"an.txt\",\"entryType\":\"-\",\"size\":3,\"modificationTime\":1660764366,"+
		"\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"}", s.buf.String())

	input2 := "i kali 0 1660763446 flag\n"
	_, err = s.writer.Write([]byte(input2))
	s.NoError(err)
	s.writer.Close()
	s.Equal("{\"files\": [{\"name\":\"an.txt\",\"entryType\":\"-\",\"size\":3,\"modificationTime\":1660764366,"+
		"\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"},"+
		"{\"name\":\"flag\",\"entryType\":\"-\",\"size\":0,\"modificationTime\":1660763446,"+
		"\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"}]}", s.buf.String())
}

func (s *Ls2JsonTestSuite) TestLs2JsonWriter_WriteLink() {
	input1 := "total 4\nlrw-rw-r-- 1 kali kali 3 1660764366 another.txt -> /bin/bash\n"
	_, err := s.writer.Write([]byte(input1))
	s.NoError(err)
	s.writer.Close()
	s.Equal("{\"files\": [{\"name\":\"another.txt\",\"entryType\":\"l\",\"linkTarget\":\"/bin/bash\",\"size\":3,"+
		"\"modificationTime\":1660764366,\"permissions\":\"rw-rw-r--\",\"owner\":\"kali\",\"group\":\"kali\"}]}",
		s.buf.String())
}
