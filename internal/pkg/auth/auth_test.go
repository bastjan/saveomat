package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bastjan/saveomat/internal/pkg/auth"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

var e = base64.URLEncoding.EncodeToString

var testAuthConf = `{
	"auths": {
		"test.io": {
			"auth": "` + e([]byte("test:test")) + `"
		},
		"https://index.docker.io/v1/": {
			"auth": "` + e([]byte("docker:docker")) + `"
		}
	}
}`

func TestRegistryAuthFor(t *testing.T) {
	subject, err := auth.FromReader(strings.NewReader(testAuthConf))
	assert.NoError(t, err)

	// The official registry is a special case. The authentication is stored under th key "https://index.docker.io/v1/"
	rAuth, err := auth.RegistryAuthFor(subject, "busybox")
	assert.NoError(t, err)
	assert.Equal(t, types.AuthConfig{Username: "docker", Password: "docker"}, decodeAuth64(t, rAuth))

	rAuth, err = auth.RegistryAuthFor(subject, "test.io/busybox")
	assert.NoError(t, err)
	assert.Equal(t, types.AuthConfig{Username: "test", Password: "test"}, decodeAuth64(t, rAuth))

	rAuth, err = auth.RegistryAuthFor(subject, "open.io/busybox")
	assert.NoError(t, err)
	assert.Equal(t, types.AuthConfig{}, decodeAuth64(t, rAuth))
}

func TestRegistryAuthForEmptyAuthenticator(t *testing.T) {
	subject := auth.EmptyAuthenticator

	rAuth, err := auth.RegistryAuthFor(subject, "busybox")
	assert.NoError(t, err)
	assert.Equal(t, types.AuthConfig{}, decodeAuth64(t, rAuth))
}

func decodeAuth64(t *testing.T, s string) types.AuthConfig {
	t.Helper()

	buf, err := base64.URLEncoding.DecodeString(s)
	assert.NoError(t, err)

	var ac types.AuthConfig
	assert.NoError(t, json.Unmarshal(buf, &ac))
	return ac
}
