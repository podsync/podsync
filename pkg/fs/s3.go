package fs

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// S3Config is the configuration for a S3-compatible storage provider
type S3Config struct {
	// S3 Bucket to store files
	Bucket string `toml:"bucket"`
	// Region of the S3 service
	Region string `toml:"region"`
	// EndpointURL is an HTTP endpoint of the S3 API
	EndpointURL string `toml:"endpoint_url"`
	// Prefix is a prefix (subfolder) to use to build key names
	Prefix string `toml:"prefix"`
}

// S3 implements file storage for S3-compatible providers.
type S3 struct {
	api      s3iface.S3API
	uploader *s3manager.Uploader
	bucket   string
	prefix   string
}

func NewS3(c S3Config) (*S3, error) {
	cfg := aws.NewConfig().
		WithEndpoint(c.EndpointURL).
		WithRegion(c.Region).
		WithLogger(s3logger{}).
		WithLogLevel(aws.LogDebug)
	sess, err := session.NewSessionWithOptions(session.Options{Config: *cfg})
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize S3 session")
	}
	return &S3{
		api:      s3.New(sess),
		uploader: s3manager.NewUploader(sess),
		bucket:   c.Bucket,
		prefix:   c.Prefix,
	}, nil
}

func (s *S3) Open(_ string) (http.File, error) {
	return nil, errors.New("serving files from S3 is not supported")
}

func (s *S3) Delete(ctx context.Context, name string) error {
	key := s.buildKey(name)
	_, err := s.api.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	return err
}

func (s *S3) Create(ctx context.Context, name string, reader io.Reader) (int64, error) {
	key := s.buildKey(name)
	logger := log.WithField("key", key)

	logger.Infof("uploading file to %s", s.bucket)
	r := &readerWithN{Reader: reader}
	_, err := s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &s.bucket,
		Key:    &key,
		Body:   r,
	})
	if err != nil {
		return 0, errors.Wrap(err, "failed to upload file")
	}

	logger.Debugf("written %d bytes", r.n)
	return int64(r.n), nil
}

func (s *S3) Size(ctx context.Context, name string) (int64, error) {
	key := s.buildKey(name)
	logger := log.WithField("key", key)

	logger.Debugf("getting file size from %s", s.bucket)
	resp, err := s.api.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == "NotFound" {
				return 0, os.ErrNotExist
			}
		}
		return 0, errors.Wrap(err, "failed to get file size")
	}

	return *resp.ContentLength, nil
}

func (s *S3) buildKey(name string) string {
	return path.Join(s.prefix, name)
}

type readerWithN struct {
	io.Reader
	n int
}

func (r *readerWithN) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	r.n += n
	return
}

type s3logger struct{}

func (s s3logger) Log(args ...interface{}) {
	log.Debug(args...)
}
