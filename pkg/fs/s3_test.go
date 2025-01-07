package fs

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/stretchr/testify/assert"
)

func TestS3_Create(t *testing.T) {
	files := make(map[string][]byte)
	stor, err := newMockS3(files, "")
	assert.NoError(t, err)

	written, err := stor.Create(testCtx, "1/test", bytes.NewBuffer([]byte{1, 5, 7, 8, 3}))
	assert.NoError(t, err)
	assert.EqualValues(t, 5, written)

	d, ok := files["1/test"]
	assert.True(t, ok)
	assert.EqualValues(t, 5, len(d))
}

func TestS3_Size(t *testing.T) {
	files := make(map[string][]byte)
	stor, err := newMockS3(files, "")
	assert.NoError(t, err)

	_, err = stor.Create(testCtx, "1/test", bytes.NewBuffer([]byte{1, 5, 7, 8, 3}))
	assert.NoError(t, err)

	sz, err := stor.Size(testCtx, "1/test")
	assert.NoError(t, err)
	assert.EqualValues(t, 5, sz)
}

func TestS3_NoSize(t *testing.T) {
	files := make(map[string][]byte)
	stor, err := newMockS3(files, "")
	assert.NoError(t, err)

	_, err = stor.Size(testCtx, "1/test")
	assert.True(t, os.IsNotExist(err))
}

func TestS3_Delete(t *testing.T) {
	files := make(map[string][]byte)
	stor, err := newMockS3(files, "")
	assert.NoError(t, err)

	_, err = stor.Create(testCtx, "1/test", bytes.NewBuffer([]byte{1, 5, 7, 8, 3}))
	assert.NoError(t, err)

	err = stor.Delete(testCtx, "1/test")
	assert.NoError(t, err)

	_, err = stor.Size(testCtx, "1/test")
	assert.True(t, os.IsNotExist(err))

	_, ok := files["1/test"]
	assert.False(t, ok)
}

func TestS3_BuildKey(t *testing.T) {
	files := make(map[string][]byte)

	stor, _ := newMockS3(files, "")
	key := stor.buildKey("test-fn")
	assert.EqualValues(t, "test-fn", key)

	stor, _ = newMockS3(files, "mock-prefix")
	key = stor.buildKey("test-fn")
	assert.EqualValues(t, "mock-prefix/test-fn", key)
}

type mockS3API struct {
	s3iface.S3API
	files map[string][]byte
}

func newMockS3(files map[string][]byte, prefix string) (*S3, error) {
	api := &mockS3API{files: files}
	return &S3{
		api:      api,
		uploader: s3manager.NewUploaderWithClient(api),
		bucket:   "mock-bucket",
		prefix:   prefix,
	}, nil
}

func (m *mockS3API) PutObjectRequest(input *s3.PutObjectInput) (*request.Request, *s3.PutObjectOutput) {
	content, _ := io.ReadAll(input.Body)
	req := request.New(aws.Config{}, metadata.ClientInfo{}, request.Handlers{}, nil, &request.Operation{}, nil, nil)
	m.files[*input.Key] = content
	return req, &s3.PutObjectOutput{}
}

func (m *mockS3API) HeadObjectWithContext(_ aws.Context, input *s3.HeadObjectInput, _ ...request.Option) (*s3.HeadObjectOutput, error) {
	if _, ok := m.files[*input.Key]; ok {
		return &s3.HeadObjectOutput{ContentLength: aws.Int64(int64(len(m.files[*input.Key])))}, nil
	}
	return nil, awserr.New("NotFound", "", nil)
}

func (m *mockS3API) DeleteObjectWithContext(_ aws.Context, input *s3.DeleteObjectInput, _ ...request.Option) (*s3.DeleteObjectOutput, error) {
	if _, ok := m.files[*input.Key]; ok {
		delete(m.files, *input.Key)
		return &s3.DeleteObjectOutput{}, nil
	}
	return nil, awserr.New("NotFound", "", nil)
}
