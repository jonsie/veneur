package veneur

import (
	"compress/gzip"
	"encoding/csv"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/stretchr/testify/assert"
)

const S3TestBucket = "stripe-test-veneur"

type mockS3Client struct {
	s3iface.S3API
	putObject func(*s3.PutObjectInput) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return m.putObject(input)
}

// stubS3 sets svc to a mockS3Client that will return 200 for all responses
// useful for avoiding erroneous error log lines when testing things that aren't
// related to s3.
func stubS3() *S3Plugin {
	client := &mockS3Client{}
	client.putObject = func(*s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	}
	svc := client
	return &S3Plugin{logger: log, svc: svc}
}

// TestS3Post tests that we can correctly post a sequence of
// DDMetrics to S3
func TestS3Post(t *testing.T) {
	const Comma = '\t'
	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	client := &mockS3Client{}
	f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "2016", "10", "13", "1476370612.tsv.gz"))
	assert.NoError(t, err)
	defer f.Close()

	client.putObject = func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		// The data should be a gzipped TSV
		gzr, err := gzip.NewReader(input.Body)
		assert.NoError(t, err)
		csvr := csv.NewReader(gzr)
		csvr.Comma = Comma
		records, err := csvr.ReadAll()
		assert.NoError(t, err)

		assert.Equal(t, 6, len(records))
		assert.Equal(t, "a.b.c.max", records[0][0])
		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	}

	s3p := &S3Plugin{logger: log, svc: client}

	err = s3p.s3Post("testbox", f, tsvFt)
	assert.NoError(t, err)
}

func TestS3Path(t *testing.T) {
	const hostname = "testingbox-9f23c"

	start := time.Now()

	path := s3Path(hostname, jsonFt)

	end := time.Now()

	// We expect paths to follow this format
	// <year>/<month/<day>/<hostname>/<timestamp>.json
	// so we should be able to parse the output with this expectation
	results := strings.Split(*path, "/")
	assert.Equal(t, 5, len(results), "Expected %#v to contain 5 parts", results)

	year, err := strconv.Atoi(results[0])
	assert.NoError(t, err)
	month, err := strconv.Atoi(results[1])
	assert.NoError(t, err)
	day, err := strconv.Atoi(results[2])
	assert.NoError(t, err)

	hostname2 := results[3]
	filename := results[4]
	timestamp, err := strconv.ParseInt(strings.Split(filename, ".")[0], 10, 64)
	assert.NoError(t, err)

	sameYear := year == int(time.Now().Year()) ||
		year == int(start.Year())
	sameMonth := month == int(time.Now().Month()) ||
		month == int(start.Month())
	sameDay := day == int(time.Now().Day()) ||
		day == int(start.Day())

	// we may have started the tests a split-second before midnight
	assert.True(t, sameYear, "Expected year %s and received %s", start.Year(), year)
	assert.True(t, sameMonth, "Expected month %s and received %s", start.Month(), month)
	assert.True(t, sameDay, "Expected day %d and received %s", start.Day(), day)

	assert.Equal(t, hostname, hostname2)
	assert.True(t, start.Unix() <= timestamp && timestamp <= end.Unix())
}

func TestS3PostNoCredentials(t *testing.T) {
	s3p := &S3Plugin{logger: log, svc: nil}

	f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "2016", "10", "07", "1475863542.json"))
	assert.NoError(t, err)
	defer f.Close()

	// this should not panic
	err = s3p.s3Post("testbox", f, jsonFt)
	assert.Equal(t, S3ClientUninitializedError, err)
}
