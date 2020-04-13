//go:generate mockgen -package server -destination image_api_client_mock_test.go github.com/docker/docker/client ImageAPIClient

package server

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestPostTar(t *testing.T) {
	images := []string{"busybox", "alpine"}

	subject := NewServer(dockerMockFor(t, images))

	upload := new(bytes.Buffer)
	mpw := multipart.NewWriter(upload)
	fw, err := mpw.CreateFormFile("images.txt", "images.txt")
	assert.NoError(t, err)
	fw.Write([]byte(strings.Join(images, "\n")))
	mpw.Close()

	req := httptest.NewRequest(http.MethodPost, "/tar", upload)
	req.Header.Set(echo.HeaderContentType, mpw.FormDataContentType())
	rec := httptest.NewRecorder()
	c := subject.NewContext(req, rec)

	assert.NoError(t, subject.postTar(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	responseTar, err := ioutil.ReadAll(rec.Body)
	assert.NoError(t, err)
	expected := mockTarBytes(t)
	assert.Equal(t, expected, responseTar)
}

func TestGetTar(t *testing.T) {
	images := []string{"busybox", "alpine"}

	subject := NewServer(dockerMockFor(t, images))

	params := url.Values{"image": images}.Encode()
	req := httptest.NewRequest(http.MethodGet, "/tar?"+params, nil)
	rec := httptest.NewRecorder()
	c := subject.NewContext(req, rec)

	assert.NoError(t, subject.getTar(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	responseTar, err := ioutil.ReadAll(rec.Body)
	assert.NoError(t, err)
	expected := mockTarBytes(t)
	assert.Equal(t, expected, responseTar)
}

func dockerMockFor(t *testing.T, images []string) *MockImageAPIClient {
	ctrl := gomock.NewController(t)
	mc := NewMockImageAPIClient(ctrl)

	for _, img := range images {
		mc.
			EXPECT().
			ImagePull(
				gomock.Any(),
				gomock.Eq(img),
				gomock.Eq(types.ImagePullOptions{})).
			Return(mockProgessReader(), nil)
	}

	mc.
		EXPECT().
		ImageSave(gomock.Any(), gomock.Eq(images)).
		Return(mockTarReader(t), nil)

	return mc
}

func mockProgessReader() io.ReadCloser {
	return ioutil.NopCloser(strings.NewReader(`{}`))
}

func mockTarReader(t *testing.T) io.ReadCloser {
	t.Helper()

	content := "test tar"

	b := new(bytes.Buffer)
	tw := tar.NewWriter(b)
	tw.WriteHeader(&tar.Header{
		Name: "images",
		Size: int64(len(content)),
	})
	_, err := tw.Write([]byte(content))
	assert.NoError(t, err)
	assert.NoError(t, tw.Close())

	return ioutil.NopCloser(b)
}

func mockTarBytes(t *testing.T) []byte {
	t.Helper()
	b, err := ioutil.ReadAll(mockTarReader(t))
	assert.NoError(t, err)

	return b
}
