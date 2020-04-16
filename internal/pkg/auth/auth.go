package auth

import (
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
)

const (
	// defaultAuthKey is the key used for dockerhub in config files, which
	// is hardcoded for historical reasons.
	// https://github.com/moby/moby/blob/fc01c2b481097a6057bec3cd1ab2d7b4488c50c4/registry/config.go#L397-L404
	defaultRegistry       = "docker.io"
	defaultLegacyRegistry = "index.docker.io"
	defaultAuthKey        = "https://index.docker.io/v1/"
)

type Authenticator interface {
	GetAuthConfig(registryHostname string) (types.AuthConfig, error)
}

var EmptyAuthenticator Authenticator = emptyAuthenticator{}

type emptyAuthenticator struct{}

func (_ emptyAuthenticator) GetAuthConfig(_ string) (types.AuthConfig, error) {
	return types.AuthConfig{}, nil
}

type dockerCliConfigFileWrapper struct {
	*configfile.ConfigFile
}

func (w dockerCliConfigFileWrapper) GetAuthConfig(registryHostname string) (types.AuthConfig, error) {
	c, err := w.ConfigFile.GetAuthConfig(registryHostname)
	wrapped := types.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		Auth:          c.Auth,
		IdentityToken: c.IdentityToken,
		RegistryToken: c.RegistryToken,
	}
	return wrapped, err
}

func FromConfigFile(c *configfile.ConfigFile) Authenticator {
	return dockerCliConfigFileWrapper{c}
}

func FromReader(r io.Reader) (Authenticator, error) {
	c, err := config.LoadFromReader(r)
	if err != nil {
		return nil, err
	}
	return FromConfigFile(c), nil
}

func RegistryAuthFor(a Authenticator, image string) (string, error) {
	distributionRef, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", err
	}

	authKey := reference.Domain(distributionRef)
	if authKey == defaultRegistry || authKey == defaultLegacyRegistry {
		authKey = defaultAuthKey
	}

	aa, err := a.GetAuthConfig(authKey)
	if err != nil {
		return "", err
	}

	return encodeAuthToBase64(aa)
}

// encodeAuthToBase64 serializes the auth configuration as JSON base64 payload
func encodeAuthToBase64(authConfig types.AuthConfig) (string, error) {
	buf, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}
