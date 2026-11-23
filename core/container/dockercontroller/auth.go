/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/registry"
)

const dockerConfigEnv = "DOCKER_CONFIG"

type dockerConfigFile struct {
	CredsStore  string                         `json:"credsStore"`
	CredHelpers map[string]string              `json:"credHelpers"`
	AuthConfigs map[string]registry.AuthConfig `json:"auths"`
}

// LoadDockerAuthConfigs loads registry credentials from the Docker CLI config.
// It supports config.json and the legacy .dockercfg format.
func LoadDockerAuthConfigs() (map[string]registry.AuthConfig, error) {
	if configDir := os.Getenv(dockerConfigEnv); configDir != "" {
		return loadDockerAuthConfigs(configDir, "")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		dockerLogger.Warn("Could not determine the home directory; skipping Docker credential lookup. Set DOCKER_CONFIG to a directory containing config.json")
		return nil, nil
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
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("read Docker config %s: %w", path, err)
	}

	authConfigs := map[string]registry.AuthConfig{}
	if len(bytes.TrimSpace(configJSON)) == 0 {
		return authConfigs, true, nil
	}

	if legacy {
		err = json.Unmarshal(configJSON, &authConfigs)
	} else {
		configFile := &dockerConfigFile{}
		err = json.Unmarshal(configJSON, configFile)
		if err == nil && (configFile.CredsStore != "" || len(configFile.CredHelpers) > 0) {
			dockerLogger.Warn("Docker credential helpers are unsupported for chaincode builds; only inline credentials will be used")
		}
		authConfigs = configFile.AuthConfigs
	}
	if err != nil {
		return nil, true, fmt.Errorf("parse Docker config %s: %w", path, err)
	}

	for address, authConfig := range authConfigs {
		if authConfig.Auth != "" {
			credentials, err := base64.StdEncoding.DecodeString(authConfig.Auth)
			username, password, found := strings.Cut(string(credentials), ":")
			if err != nil || !found || username == "" {
				dockerLogger.Warnf("Skipping malformed credentials for registry %q", address)
				delete(authConfigs, address)
				continue
			}
			authConfig.Username = username
			authConfig.Password = strings.Trim(password, "\x00")
			authConfig.Auth = ""
		}
		if authConfig.Username == "" && authConfig.IdentityToken == "" && authConfig.RegistryToken == "" {
			delete(authConfigs, address)
			continue
		}
		dockerLogger.Debugf("Loaded credentials for registry %q", address)
		authConfig.ServerAddress = address
		authConfigs[address] = authConfig
	}

	return authConfigs, true, nil
}
