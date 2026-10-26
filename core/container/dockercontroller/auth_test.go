/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/registry"
	"github.com/stretchr/testify/require"
)

func TestLoadDockerAuthConfigs(t *testing.T) {
	t.Run("config.json", func(t *testing.T) {
		homeDir := t.TempDir()
		configDir := filepath.Join(homeDir, ".docker")
		require.NoError(t, os.MkdirAll(configDir, 0o700))
		auth := base64.StdEncoding.EncodeToString([]byte("alice:secret"))
		config := `{"auths":{"registry.example.com":{"auth":"` + auth + `"},"token.example.com":{"identitytoken":"token"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0o600))

		authConfigs, err := loadDockerAuthConfigs(configDir, homeDir)
		require.NoError(t, err)
		require.Equal(t, map[string]registry.AuthConfig{
			"registry.example.com": {
				Username:      "alice",
				Password:      "secret",
				ServerAddress: "registry.example.com",
			},
			"token.example.com": {
				IdentityToken: "token",
				ServerAddress: "token.example.com",
			},
		}, authConfigs)
	})

	t.Run("legacy .dockercfg", func(t *testing.T) {
		homeDir := t.TempDir()
		auth := base64.StdEncoding.EncodeToString([]byte("bob:password"))
		config := `{"legacy.example.com":{"auth":"` + auth + `"}}`
		require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".dockercfg"), []byte(config), 0o600))

		authConfigs, err := loadDockerAuthConfigs(filepath.Join(homeDir, ".docker"), homeDir)
		require.NoError(t, err)
		require.Equal(t, map[string]registry.AuthConfig{
			"legacy.example.com": {
				Username:      "bob",
				Password:      "password",
				ServerAddress: "legacy.example.com",
			},
		}, authConfigs)
	})

	t.Run("config.json takes precedence", func(t *testing.T) {
		homeDir := t.TempDir()
		configDir := filepath.Join(homeDir, ".docker")
		require.NoError(t, os.MkdirAll(configDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"auths":{}}`), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".dockercfg"), []byte(`{"legacy.example.com":{"username":"bob"}}`), 0o600))

		authConfigs, err := loadDockerAuthConfigs(configDir, homeDir)
		require.NoError(t, err)
		require.Empty(t, authConfigs)
	})

	t.Run("missing config", func(t *testing.T) {
		homeDir := t.TempDir()

		authConfigs, err := loadDockerAuthConfigs(filepath.Join(homeDir, ".docker"), homeDir)
		require.NoError(t, err)
		require.Empty(t, authConfigs)
	})

	t.Run("malformed config", func(t *testing.T) {
		homeDir := t.TempDir()
		configDir := filepath.Join(homeDir, ".docker")
		require.NoError(t, os.MkdirAll(configDir, 0o700))
		configPath := filepath.Join(configDir, "config.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{"auths":`), 0o600))

		_, err := loadDockerAuthConfigs(configDir, homeDir)
		require.ErrorContains(t, err, "parse Docker config "+configPath)
	})

	t.Run("invalid encoded credentials", func(t *testing.T) {
		homeDir := t.TempDir()
		configDir := filepath.Join(homeDir, ".docker")
		require.NoError(t, os.MkdirAll(configDir, 0o700))
		configPath := filepath.Join(configDir, "config.json")
		config := `{"auths":{"registry.example.com":{"auth":"not-base64"}}}`
		require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

		_, err := loadDockerAuthConfigs(configDir, homeDir)
		require.ErrorContains(t, err, "decode credentials for registry registry.example.com")
		require.NotContains(t, err.Error(), "not-base64")
	})
}

func TestLoadDockerAuthConfigsUsesDockerConfigEnv(t *testing.T) {
	homeDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv(dockerConfigEnv, configDir)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"auths":{"registry.example.com":{"username":"alice","password":"secret"}}}`), 0o600))

	authConfigs, err := LoadDockerAuthConfigs()
	require.NoError(t, err)
	require.Equal(t, "alice", authConfigs["registry.example.com"].Username)
	require.Equal(t, "secret", authConfigs["registry.example.com"].Password)
}

func TestLoadDockerAuthConfigsDoesNotRequireHomeWithDockerConfigEnv(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv(dockerConfigEnv, configDir)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"auths":{}}`), 0o600))

	authConfigs, err := LoadDockerAuthConfigs()
	require.NoError(t, err)
	require.Empty(t, authConfigs)
}
