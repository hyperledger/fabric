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

// This file manages public trust related artifacts related to a distributed trust store.
// It allows multiple certificate pools to be facts related to a distributed trust store.

package trust

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// SetMyECACert sets the ECA certificate
func SetMyECACert(cert []byte) {
	getMgr().AddToCertPool("transaction", cert)
}

// SetMyTCACert sets the TCA certificate
func SetMyTCACert(cert []byte) {
	getMgr().AddToCertPool("transaction", cert)
}

// SetMyTLSCert sets the TLS certificate
func SetMyTLSCert(cert []byte) {
	getMgr().AddToCertPool("tls", cert)
}

// GetTransactionCertPool returns the X509 certificate pool for transactions
func GetTransactionCertPool() *x509.CertPool {
	return getMgr().GetCertPool("transaction")
}

// GetTLSCertPool returns the X509 certificate pool for TLS
func GetTLSCertPool() *x509.CertPool {
	return getMgr().GetCertPool("tls")
}

// SetMyPeerCert sets the peer's certificate
func SetMyPeerCert(peerCert []byte) {
	getMgr().SetCertByID(getMgr().id, peerCert)
}

// GetPeerCertByID returns a peer certificate given the peer ID
func GetPeerCertByID(peerID string) *x509.Certificate {
	rawCert := getMgr().GetCertByID(peerID)
	if rawCert == nil {
		logger.Debugf("certificate for peer %s was not found", peerID)
		return nil
	}
	cert, err := primitives.PEMtoCertificate(rawCert)
	if err != nil {
		logger.Errorf("invalid cert for peer %s: %s", peerID, err.Error())
		return nil
	}
	return cert
}

func getMgr() *Mgr {
	if mgr == nil {
		id := viper.GetString("peer.id")
		dir := filepath.Join(viper.GetString("peer.fileSystemPath"), "trust")
		InitMgr(id, dir)
	}
	return mgr
}

var mgr *Mgr

// InitMgr initializes the trust manager
func InitMgr(id, dir string) {
	mgr = NewMgr(id, dir)
	logger.Debugf("initialized trust manager for %s at %s", id, dir)
}

// GetMgr returns my trust manager.  It panics if InitMgr has not been called.
func GetMgr() *Mgr {
	if mgr == nil {
		logger.Panic("the trust manager has not been initialized")
	}
	return mgr
}

// Mgr is a file-based public trust manager to support decentralization
type Mgr struct {
	// The identifier for the local trust mgr
	id string
	// The directory containing all trust stores
	dir string
	// Path to the file containing the trust info I'm contributing
	myTrustFile string
	// My trust file info
	myTrustInfo *PerFileInfo
	// X509 certificates
	certs map[string][]byte
	// X509 certificate pools
	pools map[string]*x509.CertPool
}

// PerFileInfo contains what is contributed by each Mgr
type PerFileInfo struct {
	Certs map[string]string   `json:"certs"`
	Pools map[string][]string `json:"pools"`
}

var logger = logging.MustGetLogger("trust")

// NewMgr is the constructor for a Mgr
func NewMgr(id, dir string) *Mgr {
	mgr := &Mgr{id: id, dir: dir}
	mgr.myTrustFile = filepath.Join(dir, id+".json")
	mgr.myTrustInfo = newPerFileInfo()
	mgr.certs = make(map[string][]byte)
	mgr.pools = make(map[string]*x509.CertPool)
	mgr.load()
	return mgr
}

// GetCertPool returns a certificate pool which was created using AddCertToPool.
// Returns nil if the pool does not exist.
func (mgr *Mgr) GetCertPool(poolName string) *x509.CertPool {
	return mgr.pools[poolName]
}

// AddToCertPool adds a certificate to an X509 certificate pool.
// It creates the pool if it doesn't exist
func (mgr *Mgr) AddToCertPool(poolName string, cert []byte) {
	mgr.localAddToCertPool(poolName, cert)
	mgr.globalAddToCertPool(poolName, cert)
}

// GetCertByID returns a certificate which was stored by ID, or nil if not found.
func (mgr *Mgr) GetCertByID(certID string) []byte {
	return mgr.certs[certID]
}

// SetCertByID sets a certificate by an ID.
func (mgr *Mgr) SetCertByID(certID string, cert []byte) {
	mgr.localSetCertByID(certID, cert)
	mgr.globalSetCertByID(certID, cert)
}

func (mgr *Mgr) localAddToCertPool(poolName string, cert []byte) {
	pool := mgr.myTrustInfo.Pools[poolName]
	if pool == nil {
		pool = make([]string, 0)
	}
	mgr.myTrustInfo.Pools[poolName] = append(pool, b64Encode(cert))
	mgr.store()
}

func (mgr *Mgr) globalAddToCertPool(poolName string, cert []byte) {
	pool := mgr.pools[poolName]
	if pool == nil {
		pool = x509.NewCertPool()
		mgr.pools[poolName] = pool
	}
	x509Cert, err := x509.ParseCertificate(cert)
	if err != nil {
		logger.Errorf("unable to add certificate to %s pool: %s", poolName, err.Error())
	} else {
		pool.AddCert(x509Cert)
		logger.Errorf("successfully added cert to pool %s", poolName)
	}
}

func (mgr *Mgr) localSetCertByID(certID string, cert []byte) {
	mgr.myTrustInfo.Certs[certID] = b64Encode(cert)
	mgr.store()
}

func (mgr *Mgr) globalSetCertByID(certID string, cert []byte) {
	mgr.certs[certID] = cert
}

// Load all files in the trust directory
func (mgr *Mgr) load() {
	logger.Debugf("loading trust directory %s ...", mgr.dir)
	if _, err := os.Stat(mgr.dir); os.IsNotExist(err) {
		err = os.Mkdir(mgr.dir, 0755)
		if err != nil {
			logger.Panic(fmt.Sprintf("trust: Error creating directory %s: %s", mgr.dir, err))
		}
	}
	err := filepath.Walk(mgr.dir, mgr.visit)
	if err != nil {
		logger.Panic(fmt.Sprintf("trust: Error walking directory %s: %s", mgr.dir, err))
	}
	logger.Debugf("successfully loaded trust directory %s", mgr.dir)
}

func (mgr *Mgr) visit(path string, file os.FileInfo, err error) error {
	if file.IsDir() {
		return nil
	}
	if path == mgr.myTrustFile {
		return nil
	}
	return mgr.loadTrustFile(path)
}

func (mgr *Mgr) loadTrustFile(path string) error {
	logger.Debugf("loading trust file %s ...", path)
	// Open the file
	file, err := os.Open(path)
	if err != nil {
		logger.Errorf("error opening trust file %s: [%s]", path, err.Error())
		return err
	}
	defer file.Close()
	// Parse the JSON
	jsonParser := json.NewDecoder(file)
	var info PerFileInfo
	if err = jsonParser.Decode(&info); err != nil {
		logger.Errorf("error parsing trust file: [%s]", err.Error())
		return err
	}
	// Set certificates by ID
	for id, cert := range info.Certs {
		mgr.globalSetCertByID(id, b64Decode(cert))
	}
	// Add each certificate to the correct cert pool
	for name, pool := range info.Pools {
		for _, cert := range pool {
			mgr.globalAddToCertPool(name, b64Decode(cert))
		}
	}
	// Successfully
	logger.Debugf("sucessfully loaded trust file %s", path)
	return nil
}

// Save my trust info to my trust file
func (mgr *Mgr) store() {
	info, err := json.MarshalIndent(mgr.myTrustInfo, "", "   ")
	if err != nil {
		logger.Errorf("store: failed marshalling my trust: %s", err.Error())
		return
	}
	err = ioutil.WriteFile(mgr.myTrustFile, info, 0644)
	if err != nil {
		logger.Errorf("store: failed writing to %s: %s", mgr.myTrustFile, err.Error())
		return
	}
	logger.Debugf("successfully updated %s", mgr.myTrustFile)
}

func newPerFileInfo() *PerFileInfo {
	return &PerFileInfo{Certs: make(map[string]string), Pools: make(map[string][]string)}
}

// Base 64 encode bytes to a string
func b64Encode(bytes []byte) string {
	return base64.StdEncoding.EncodeToString([]byte(bytes))
}

// Base 64 decode
func b64Decode(str string) []byte {
	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		logger.Errorf("failed decoding string: %s", err.Error())
		bytes = make([]byte, 0)
	}
	return bytes
}
