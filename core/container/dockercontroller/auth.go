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
	AuthConfigs map[string]registry.AuthConfig `json:"auths"`
}

// LoadDockerAuthConfigs loads registry credentials from the Docker CLI config.
// It supports config.json and the legacy .dockercfg format.
func LoadDockerAuthConfigs() (map[string]registry.AuthConfig, error) {
	configDir := os.Getenv(dockerConfigEnv)
	homeDir, err := os.UserHomeDir()
	if err != nil && configDir == "" {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}

	if configDir == "" {
		configDir = filepath.Join(homeDir, ".docker")
	}

	return loadDockerAuthConfigs(configDir, homeDir)
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
		authConfigs = configFile.AuthConfigs
	}
	if err != nil {
		return nil, true, fmt.Errorf("parse Docker config %s: %w", path, err)
	}

	for address, authConfig := range authConfigs {
		if authConfig.Auth != "" {
			credentials, err := base64.StdEncoding.DecodeString(authConfig.Auth)
			if err != nil {
				return nil, true, fmt.Errorf("decode credentials for registry %s in Docker config %s: %w", address, path, err)
			}

			username, password, found := strings.Cut(string(credentials), ":")
			if !found || username == "" {
				return nil, true, fmt.Errorf("invalid credentials for registry %s in Docker config %s", address, path)
			}
			authConfig.Username = username
			authConfig.Password = strings.Trim(password, "\x00")
			authConfig.Auth = ""
		}
		authConfig.ServerAddress = address
		authConfigs[address] = authConfig
	}

	return authConfigs, true, nil
}
