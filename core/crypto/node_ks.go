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

package crypto

import (
	"crypto/x509"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperledger/fabric/core/crypto/utils"

	// Required to successfully initialized the driver
	"github.com/hyperledger/fabric/core/crypto/primitives"
	_ "github.com/mattn/go-sqlite3"
)

/*
var (
	defaultCerts = make(map[string][]byte)
)

func addDefaultCert(key string, cert []byte) error {
	log.Debugf("Adding Default Cert [%s][%s]", key, utils.EncodeBase64(cert))

	der, err := utils.PEMtoDER(cert)
	if err != nil {
		log.Errorf("Failed adding default cert: [%s]", err)

		return err
	}

	defaultCerts[key] = der

	return nil
}
*/

func (node *nodeImpl) initKeyStore(pwd []byte) error {
	ks := keyStore{}
	if err := ks.init(node, pwd); err != nil {
		return err
	}
	node.ks = &ks

	/*
		// Add default certs
		for key, value := range defaultCerts {
			node.debug("Adding Default Cert to the keystore [%s][%s]", key, utils.EncodeBase64(value))
			ks.storeCert(key, value)
		}
	*/

	return nil
}

type keyStore struct {
	node *nodeImpl

	isOpen bool

	pwd []byte

	// backend
	sqlDB *sql.DB

	// Sync
	m sync.Mutex
}

func (ks *keyStore) init(node *nodeImpl, pwd []byte) error {
	ks.m.Lock()
	defer ks.m.Unlock()

	if ks.isOpen {
		return utils.ErrKeyStoreAlreadyInitialized
	}

	ks.node = node
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

func (ks *keyStore) isAliasSet(alias string) bool {
	missing, _ := utils.FilePathMissing(ks.node.conf.getPathForAlias(alias))
	if missing {
		return false
	}

	return true
}

func (ks *keyStore) storePrivateKey(alias string, privateKey interface{}) error {
	rawKey, err := primitives.PrivateKeyToPEM(privateKey, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed converting private key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.node.conf.getPathForAlias(alias), rawKey, 0700)
	if err != nil {
		ks.node.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) storePrivateKeyInClear(alias string, privateKey interface{}) error {
	rawKey, err := primitives.PrivateKeyToPEM(privateKey, nil)
	if err != nil {
		ks.node.Errorf("Failed converting private key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.node.conf.getPathForAlias(alias), rawKey, 0700)
	if err != nil {
		ks.node.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) deletePrivateKeyInClear(alias string) error {
	return os.Remove(ks.node.conf.getPathForAlias(alias))
}

func (ks *keyStore) loadPrivateKey(alias string) (interface{}, error) {
	path := ks.node.conf.getPathForAlias(alias)
	ks.node.Debugf("Loading private key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := primitives.PEMtoPrivateKey(raw, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *keyStore) storePublicKey(alias string, publicKey interface{}) error {
	rawKey, err := primitives.PublicKeyToPEM(publicKey, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed converting public key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.node.conf.getPathForAlias(alias), rawKey, 0700)
	if err != nil {
		ks.node.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) loadPublicKey(alias string) (interface{}, error) {
	path := ks.node.conf.getPathForAlias(alias)
	ks.node.Debugf("Loading public key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading public key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := primitives.PEMtoPublicKey(raw, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *keyStore) storeKey(alias string, key []byte) error {
	pem, err := primitives.AEStoEncryptedPEM(key, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed converting key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.node.conf.getPathForAlias(alias), pem, 0700)
	if err != nil {
		ks.node.Errorf("Failed storing key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) loadKey(alias string) ([]byte, error) {
	path := ks.node.conf.getPathForAlias(alias)
	ks.node.Debugf("Loading key [%s] at [%s]...", alias, path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	key, err := primitives.PEMtoAES(pem, ks.pwd)
	if err != nil {
		ks.node.Errorf("Failed parsing key [%s]: [%s]", alias, err)

		return nil, err
	}

	return key, nil
}

func (ks *keyStore) storeCert(alias string, der []byte) error {
	err := ioutil.WriteFile(ks.node.conf.getPathForAlias(alias), primitives.DERCertToPEM(der), 0700)
	if err != nil {
		ks.node.Errorf("Failed storing certificate [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *keyStore) certMissing(alias string) bool {
	return !ks.isAliasSet(alias)
}

func (ks *keyStore) deleteCert(alias string) error {
	return os.Remove(ks.node.conf.getPathForAlias(alias))
}

func (ks *keyStore) loadCert(alias string) ([]byte, error) {
	path := ks.node.conf.getPathForAlias(alias)
	ks.node.Debugf("Loading certificate [%s] at [%s]...", alias, path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading certificate [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return pem, nil
}

func (ks *keyStore) loadExternalCert(path string) ([]byte, error) {
	ks.node.Debugf("Loading external certificate at [%s]...", path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading external certificate: [%s].", err.Error())

		return nil, err
	}

	return pem, nil
}

func (ks *keyStore) loadCertX509AndDer(alias string) (*x509.Certificate, []byte, error) {
	path := ks.node.conf.getPathForAlias(alias)
	ks.node.Debugf("Loading certificate [%s] at [%s]...", alias, path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		ks.node.Errorf("Failed loading certificate [%s]: [%s].", alias, err.Error())

		return nil, nil, err
	}

	cert, der, err := primitives.PEMtoCertificateAndDER(pem)
	if err != nil {
		ks.node.Errorf("Failed parsing certificate [%s]: [%s].", alias, err.Error())

		return nil, nil, err
	}

	return cert, der, nil
}

func (ks *keyStore) close() error {
	ks.node.Debug("Closing keystore...")
	err := ks.sqlDB.Close()

	if err != nil {
		ks.node.Errorf("Failed closing keystore [%s].", err.Error())
	} else {
		ks.node.Debug("Closing keystore...done!")
	}

	ks.isOpen = false
	return err
}

func (ks *keyStore) createKeyStoreIfNotExists() error {
	// Check keystore directory
	ksPath := ks.node.conf.getKeyStorePath()
	missing, err := utils.DirMissingOrEmpty(ksPath)
	ks.node.Debugf("Keystore path [%s] missing [%t]: [%s]", ksPath, missing, utils.ErrToString(err))

	if !missing {
		// Check keystore file
		missing, err = utils.FileMissing(ks.node.conf.getKeyStorePath(), ks.node.conf.getKeyStoreFilename())
		ks.node.Debugf("Keystore [%s] missing [%t]:[%s]", ks.node.conf.getKeyStoreFilePath(), missing, utils.ErrToString(err))
	}

	if missing {
		err := ks.createKeyStore()
		if err != nil {
			ks.node.Errorf("Failed creating db At [%s]: [%s]", ks.node.conf.getKeyStoreFilePath(), err.Error())
			return nil
		}
	}

	return nil
}

func (ks *keyStore) createKeyStore() error {
	// Create keystore directory root if it doesn't exist yet
	ksPath := ks.node.conf.getKeyStorePath()
	ks.node.Debugf("Creating Keystore at [%s]...", ksPath)

	missing, err := utils.FileMissing(ksPath, ks.node.conf.getKeyStoreFilename())
	if !missing {
		ks.node.Debugf("Creating Keystore at [%s]. Keystore already there", ksPath)
		return nil
	}

	os.MkdirAll(ksPath, 0755)

	// Create Raw material folder
	os.MkdirAll(ks.node.conf.getRawsPath(), 0755)

	// Create DB
	ks.node.Debug("Open Keystore DB...")
	db, err := sql.Open("sqlite3", filepath.Join(ksPath, ks.node.conf.getKeyStoreFilename()))
	if err != nil {
		return err
	}

	ks.node.Debug("Ping Keystore DB...")
	err = db.Ping()
	if err != nil {
		ks.node.Errorf("Failend pinged keystore DB: [%s]", err)

		return err
	}
	defer db.Close()

	ks.node.Debugf("Keystore created at [%s].", ksPath)
	return nil
}

func (ks *keyStore) deleteKeyStore() error {
	ks.node.Debugf("Removing KeyStore at [%s].", ks.node.conf.getKeyStorePath())

	return os.RemoveAll(ks.node.conf.getKeyStorePath())
}

func (ks *keyStore) openKeyStore() error {
	if ks.isOpen {
		return nil
	}

	// Open DB
	ksPath := ks.node.conf.getKeyStorePath()

	sqlDB, err := sql.Open("sqlite3", filepath.Join(ksPath, ks.node.conf.getKeyStoreFilename()))
	if err != nil {
		ks.node.Errorf("Error opening keystore%s", err.Error())
		return err
	}
	ks.isOpen = true
	ks.sqlDB = sqlDB

	ks.node.Debugf("Keystore opened at [%s]...done", ksPath)

	return nil
}
