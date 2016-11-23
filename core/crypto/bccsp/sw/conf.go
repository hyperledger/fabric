/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package sw

import (
	"errors"
	"path/filepath"

	"os"

	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"

	"github.com/spf13/viper"
	"golang.org/x/crypto/sha3"
)

type config struct {
	keystorePath  string
	securityLevel int
	hashFamily    string

	configurationPathProperty string
	ellipticCurve             elliptic.Curve
	hashFunction              func() hash.Hash
	aesBitLength              int
	rsaBitLength              int
}

func (conf *config) init(securityLevel int, hashFamily string) error {
	// Set security level
	err := conf.setSecurityLevel(securityLevel, hashFamily)
	if err != nil {
		return fmt.Errorf("Failed initliazing security level [%s]", err)
	}
	// Set ks path
	conf.configurationPathProperty = "security.bccsp.default.keyStorePath"

	// Check mandatory fields
	var rootPath string
	if err := conf.checkProperty(conf.configurationPathProperty); err != nil {
		logger.Warning("'security.bccsp.default.keyStorePath' not set. Using the default directory [%s] for temporary files", os.TempDir())
		rootPath = os.TempDir()
	} else {
		rootPath = viper.GetString(conf.configurationPathProperty)
	}
	logger.Infof("Root Path [%s]", rootPath)
	// Set configuration path
	rootPath = filepath.Join(rootPath, "crypto")

	conf.keystorePath = filepath.Join(rootPath, "ks")

	return nil
}

func (conf *config) setSecurityLevel(securityLevel int, hashFamily string) (err error) {
	switch hashFamily {
	case "SHA2":
		err = conf.setSecurityLevelSHA2(securityLevel)
	case "SHA3":
		err = conf.setSecurityLevelSHA3(securityLevel)
	default:
		err = fmt.Errorf("Hash Family not supported [%s]", hashFamily)
	}
	return
}

func (conf *config) setSecurityLevelSHA2(level int) (err error) {
	switch level {
	case 256:
		conf.ellipticCurve = elliptic.P256()
		conf.hashFunction = sha256.New
		conf.rsaBitLength = 2048
		conf.aesBitLength = 32
	case 384:
		conf.ellipticCurve = elliptic.P384()
		conf.hashFunction = sha512.New384
		conf.rsaBitLength = 3072
		conf.aesBitLength = 32
	default:
		err = fmt.Errorf("Security level not supported [%d]", level)
	}
	return
}

func (conf *config) setSecurityLevelSHA3(level int) (err error) {
	switch level {
	case 256:
		conf.ellipticCurve = elliptic.P256()
		conf.hashFunction = sha3.New256
		conf.rsaBitLength = 2048
		conf.aesBitLength = 32
	case 384:
		conf.ellipticCurve = elliptic.P384()
		conf.hashFunction = sha3.New384
		conf.rsaBitLength = 3072
		conf.aesBitLength = 32
	default:
		err = fmt.Errorf("Security level not supported [%d]", level)
	}
	return
}

func (conf *config) checkProperty(property string) error {
	res := viper.GetString(property)
	if res == "" {
		return errors.New("Property not specified in configuration file. Please check that property is set: " + property)
	}
	return nil
}

func (conf *config) getKeyStorePath() string {
	return conf.keystorePath
}

func (conf *config) getPathForAlias(alias, suffix string) string {
	return filepath.Join(conf.getKeyStorePath(), alias+"_"+suffix)
}
