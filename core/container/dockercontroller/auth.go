/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/registry"
	dcli "github.com/moby/moby/client"
)

const dockerConfigEnv = "DOCKER_CONFIG"

type dockerConfigFile struct {
	CredsStore  string                     `json:"credsStore"`
	CredHelpers map[string]string          `json:"credHelpers"`
	AuthConfigs map[string]json.RawMessage `json:"auths"`
}

// LoadDockerAuthConfigs loads registry credentials from the Docker CLI config.
// It supports config.json and the legacy .dockercfg format.
func LoadDockerAuthConfigs() (map[string]registry.AuthConfig, error) {
	return loadDockerAuthConfigsForUser(user.Current)
}

func loadDockerAuthConfigsForUser(currentUser func() (*user.User, error)) (map[string]registry.AuthConfig, error) {
	if configDir := os.Getenv(dockerConfigEnv); configDir != "" {
		return loadDockerAuthConfigs(configDir, "")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		account, lookupErr := currentUser()
		if lookupErr != nil || account.HomeDir == "" {
			dockerLogger.Warn("Could not determine the home directory; skipping Docker credential lookup. Set DOCKER_CONFIG to a directory containing config.json")
			return nil, nil
		}
		homeDir = account.HomeDir
	}
	return loadDockerAuthConfigs(filepath.Join(homeDir, ".docker"), homeDir)
}

func loadDockerAuthConfigs(configDir, homeDir string) (map[string]registry.AuthConfig, error) {
	configPath := filepath.Join(configDir, "config.json")
	authConfigs, found, err := readDockerAuthConfigs(configPath, false)
	if err != nil || found {
		return authConfigs, err
	}
	if homeDir == "" {
		return nil, nil
	}

	legacyAuthConfigs, _, err := readDockerAuthConfigs(filepath.Join(homeDir, ".dockercfg"), true)
	return legacyAuthConfigs, err
}

func readDockerAuthConfigs(path string, legacy bool) (map[string]registry.AuthConfig, bool, error) {
	configJSON, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			dockerLogger.Debugf("Docker credential config not found at %q", path)
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("read Docker config %s: %w", path, err)
	}

	authConfigs := map[string]registry.AuthConfig{}
	rawConfigs := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(configJSON)) == 0 {
		return authConfigs, true, nil
	}

	if legacy {
		err = json.Unmarshal(configJSON, &rawConfigs)
	} else {
		configFile := &dockerConfigFile{}
		err = json.Unmarshal(configJSON, configFile)
		if err == nil && (configFile.CredsStore != "" || len(configFile.CredHelpers) > 0) {
			dockerLogger.Warn("Docker credential helpers are unsupported for chaincode builds; only inline credentials will be used")
		}
		rawConfigs = configFile.AuthConfigs
	}
	if err != nil {
		return nil, true, fmt.Errorf("parse Docker config %s: %w", path, err)
	}

	for address, raw := range rawConfigs {
		var authConfig registry.AuthConfig
		if err := json.Unmarshal(raw, &authConfig); err != nil {
			dockerLogger.Warnf("Skipping malformed credentials for registry %q", address)
			continue
		}
		if authConfig.Auth != "" {
			credentials, err := base64.StdEncoding.DecodeString(authConfig.Auth)
			if err != nil {
				dockerLogger.Warnf("Skipping malformed credentials for registry %q", address)
				continue
			}
			username, password, found := strings.Cut(string(credentials), ":")
			if !found || username == "" {
				dockerLogger.Warnf("Skipping malformed credentials for registry %q", address)
				continue
			}
			authConfig.Username = username
			authConfig.Password = strings.Trim(password, "\x00")
			authConfig.Auth = ""
		}
		if authConfig.Username == "" && authConfig.IdentityToken == "" && authConfig.RegistryToken == "" {
			continue
		}
		dockerLogger.Debugf("Loaded credentials for registry %q", address)
		authConfig.ServerAddress = address
		authConfigs[address] = authConfig
	}

	return authConfigs, true, nil
}

// RegistryClient supplies inline credentials for platform builder image pulls.
type RegistryClient struct {
	dcli.APIClient
}

func (c *RegistryClient) ImagePull(ctx context.Context, image string, options dcli.ImagePullOptions) (dcli.ImagePullResponse, error) {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return nil, err
	}
	configs, err := LoadDockerAuthConfigs()
	if err != nil {
		dockerLogger.Warnf("Could not load Docker registry credentials: %v", err)
	}
	host := reference.Domain(ref)
	for address, auth := range configs {
		normalized := address
		if !strings.Contains(normalized, "://") {
			normalized = "https://" + normalized
		}
		u, err := url.Parse(normalized)
		if err != nil {
			continue
		}
		registryHost := u.Host
		if registryHost == "index.docker.io" {
			registryHost = "docker.io"
		}
		if registryHost == host {
			encoded, err := json.Marshal(auth)
			if err != nil {
				return nil, err
			}
			options.RegistryAuth = base64.URLEncoding.EncodeToString(encoded)
			break
		}
	}
	return c.APIClient.ImagePull(ctx, image, options)
}
