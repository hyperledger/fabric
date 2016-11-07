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
	"io/ioutil"
	"os"
	"sync"

	"github.com/hyperledger/fabric/core/crypto/utils"

	"errors"
	"strings"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type keyStore struct {
	conf *config

	isOpen bool

	pwd []byte

	// Sync
	m sync.Mutex
}

func (ks *keyStore) init(pwd []byte, conf *config) error {
	// Validate inputs
	// pwd can be nil

	if conf == nil {
		return errors.New("Invalid config. Nil.")
	}

	ks.m.Lock()
	defer ks.m.Unlock()

	if ks.isOpen {
		return errors.New("Keystore already Initilized.")
	}

	ks.conf = conf
	ks.pwd = utils.Clone(pwd)

	err := ks.createKeyStoreIfNotExists()
	if err != nil {
		return err
	}

	err = ks.openKeyStore()
	if err != nil {
		return err
	}

	return nil
}

func (ks *keyStore) getSuffix(alias string) string {
	files, _ := ioutil.ReadDir(ks.conf.getKeyStorePath())
	for _, f := range files {
		if strings.HasPrefix(f.Name(), alias) {
			if strings.HasSuffix(f.Name(), "sk") {
				return "sk"
			}
			if strings.HasSuffix(f.Name(), "pk") {
				return "pk"
			}
			if strings.HasSuffix(f.Name(), "key") {
				return "key"
			}
			break
		}
	}
	return ""
}

func (ks *keyStore) storePrivateKey(alias string, privateKey interface{}) error {
	rawKey, err := primitives.PrivateKeyToPEM(privateKey, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting private key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.conf.getPathForAlias(alias, "sk"), rawKey, 0700)
	if err != nil {
		logger.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) loadPrivateKey(alias string) (interface{}, error) {
	path := ks.conf.getPathForAlias(alias, "sk")
	logger.Debugf("Loading private key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := primitives.PEMtoPrivateKey(raw, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *keyStore) storePublicKey(alias string, publicKey interface{}) error {
	rawKey, err := primitives.PublicKeyToPEM(publicKey, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting public key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.conf.getPathForAlias(alias, "pk"), rawKey, 0700)
	if err != nil {
		logger.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) loadPublicKey(alias string) (interface{}, error) {
	path := ks.conf.getPathForAlias(alias, "pk")
	logger.Debugf("Loading public key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading public key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := primitives.PEMtoPublicKey(raw, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *keyStore) storeKey(alias string, key []byte) error {
	pem, err := primitives.AEStoEncryptedPEM(key, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.conf.getPathForAlias(alias, "key"), pem, 0700)
	if err != nil {
		logger.Errorf("Failed storing key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) loadKey(alias string) ([]byte, error) {
	path := ks.conf.getPathForAlias(alias, "key")
	logger.Debugf("Loading key [%s] at [%s]...", alias, path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	key, err := primitives.PEMtoAES(pem, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing key [%s]: [%s]", alias, err)

		return nil, err
	}

	return key, nil
}

func (ks *keyStore) close() error {
	ks.isOpen = false
	logger.Debug("Closing keystore...done!")
	return nil
}

func (ks *keyStore) createKeyStoreIfNotExists() error {
	// Check keystore directory
	ksPath := ks.conf.getKeyStorePath()
	missing, err := utils.DirMissingOrEmpty(ksPath)
	logger.Infof("Keystore path [%s] missing [%t]: [%s]", ksPath, missing, utils.ErrToString(err))

	if missing {
		err := ks.createKeyStore()
		if err != nil {
			logger.Errorf("Failed creating ks At [%s]: [%s]", ksPath, err.Error())
			return nil
		}
	}

	return nil
}

func (ks *keyStore) createKeyStore() error {
	// Create keystore directory root if it doesn't exist yet
	ksPath := ks.conf.getKeyStorePath()
	logger.Debugf("Creating Keystore at [%s]...", ksPath)

	os.MkdirAll(ksPath, 0755)

	logger.Debugf("Keystore created at [%s].", ksPath)
	return nil
}

func (ks *keyStore) deleteKeyStore() error {
	logger.Debugf("Removing KeyStore at [%s].", ks.conf.getKeyStorePath())

	return os.RemoveAll(ks.conf.getKeyStorePath())
}

func (ks *keyStore) openKeyStore() error {
	if ks.isOpen {
		return nil
	}

	logger.Debugf("Keystore opened at [%s]...done", ks.conf.getKeyStorePath())

	return nil
}
