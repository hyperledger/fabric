/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/cmd/common/comm"
	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	v12 = iota
)

// Config aggregates configuration of TLS and signing
type Config struct {
	Version      int
	TLSConfig    comm.Config
	SignerConfig signer.Config
}

// ConfigFromFile loads the given file and converts it to a Config
func ConfigFromFile(file string) (Config, error) {
	configData, err := ioutil.ReadFile(file)
	if err != nil {
		return Config{}, errors.WithStack(err)
	}
	config := Config{}

	if err := yaml.Unmarshal([]byte(configData), &config); err != nil {
		return Config{}, errors.Errorf("error unmarshaling YAML file %s: %s", file, err)
	}

	return config, validateConfig(config)
}

// ToFile writes the config into a file
func (c Config) ToFile(file string) error {
	if err := validateConfig(c); err != nil {
		return errors.Wrap(err, "config isn't valid")
	}
	b, _ := yaml.Marshal(c)
	if err := ioutil.WriteFile(file, b, 0600); err != nil {
		return errors.Errorf("failed writing file %s: %v", file, err)
	}
	return nil
}

func validateConfig(conf Config) error {
	nonEmptyStrings := []string{
		conf.SignerConfig.MSPID,
		conf.SignerConfig.IdentityPath,
		conf.SignerConfig.KeyPath,
	}

	for _, s := range nonEmptyStrings {
		if s == "" {
			return errors.New("empty string that is mandatory")
		}
	}
	return nil
}
