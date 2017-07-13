/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package msp

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"

	"encoding/hex"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/tools/cryptogen/ca"
	"github.com/hyperledger/fabric/common/tools/cryptogen/csp"
)

func GenerateLocalMSP(baseDir, name string, sans []string, signCA *ca.CA,
	tlsCA *ca.CA) error {

	// create folder structure
	mspDir := filepath.Join(baseDir, "msp")
	tlsDir := filepath.Join(baseDir, "tls")

	err := createFolderStructure(mspDir, true)
	if err != nil {
		return err
	}

	err = os.MkdirAll(tlsDir, 0755)
	if err != nil {
		return err
	}

	/*
		Create the MSP identity artifacts
	*/
	// get keystore path
	keystore := filepath.Join(mspDir, "keystore")

	// generate private key
	priv, _, err := csp.GeneratePrivateKey(keystore)
	if err != nil {
		return err
	}

	// get public key
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return err
	}
	// generate X509 certificate using signing CA
	cert, err := signCA.SignCertificate(filepath.Join(mspDir, "signcerts"),
		name, []string{}, ecPubKey, x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	if err != nil {
		return err
	}

	// write artifacts to MSP folders

	// the signing CA certificate goes into cacerts
	err = x509Export(filepath.Join(mspDir, "cacerts", x509Filename(signCA.Name)), signCA.SignCert)
	if err != nil {
		return err
	}
	// the TLS CA certificate goes into tlscacerts
	err = x509Export(filepath.Join(mspDir, "tlscacerts", x509Filename(tlsCA.Name)), tlsCA.SignCert)
	if err != nil {
		return err
	}

	// the signing identity goes into admincerts.
	// This means that the signing identity
	// of this MSP is also an admin of this MSP
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	err = x509Export(filepath.Join(mspDir, "admincerts", x509Filename(name)), cert)
	if err != nil {
		return err
	}

	/*
		Generate the TLS artifacts in the TLS folder
	*/

	// generate private key
	tlsPrivKey, _, err := csp.GeneratePrivateKey(tlsDir)
	if err != nil {
		return err
	}
	// get public key
	tlsPubKey, err := csp.GetECPublicKey(tlsPrivKey)
	if err != nil {
		return err
	}
	// generate X509 certificate using TLS CA
	_, err = tlsCA.SignCertificate(filepath.Join(tlsDir),
		name, sans, tlsPubKey, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth})
	if err != nil {
		return err
	}
	err = x509Export(filepath.Join(tlsDir, "ca.crt"), tlsCA.SignCert)
	if err != nil {
		return err
	}

	// rename the generated TLS X509 cert
	err = os.Rename(filepath.Join(tlsDir, x509Filename(name)),
		filepath.Join(tlsDir, "server.crt"))
	if err != nil {
		return err
	}

	err = keyExport(tlsDir, filepath.Join(tlsDir, "server.key"), tlsPrivKey)
	if err != nil {
		return err
	}

	return nil
}

func GenerateVerifyingMSP(baseDir string, signCA *ca.CA, tlsCA *ca.CA) error {

	// create folder structure and write artifacts to proper locations
	err := createFolderStructure(baseDir, false)
	if err == nil {
		// the signing CA certificate goes into cacerts
		err = x509Export(filepath.Join(baseDir, "cacerts", x509Filename(signCA.Name)), signCA.SignCert)
		if err != nil {
			return err
		}
		// the TLS CA certificate goes into tlscacerts
		err = x509Export(filepath.Join(baseDir, "tlscacerts", x509Filename(tlsCA.Name)), tlsCA.SignCert)
		if err != nil {
			return err
		}
	}

	// create a throwaway cert to act as an admin cert
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	factory.InitFactories(nil)
	bcsp := factory.GetDefault()
	priv, err := bcsp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: true})
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return err
	}
	_, err = signCA.SignCertificate(filepath.Join(baseDir, "admincerts"), signCA.Name,
		[]string{""}, ecPubKey, x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	if err != nil {
		return err
	}

	return nil
}

func createFolderStructure(rootDir string, local bool) error {

	var folders []string
	// create admincerts, cacerts, keystore and signcerts folders
	folders = []string{
		filepath.Join(rootDir, "admincerts"),
		filepath.Join(rootDir, "cacerts"),
		filepath.Join(rootDir, "tlscacerts"),
	}
	if local {
		folders = append(folders, filepath.Join(rootDir, "keystore"),
			filepath.Join(rootDir, "signcerts"))
	}

	for _, folder := range folders {
		err := os.MkdirAll(folder, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}

func x509Filename(name string) string {
	return name + "-cert.pem"
}

func x509Export(path string, cert *x509.Certificate) error {
	return pemExport(path, "CERTIFICATE", cert.Raw)
}

func keyExport(keystore, output string, key bccsp.Key) error {
	id := hex.EncodeToString(key.SKI())

	return os.Rename(filepath.Join(keystore, id+"_sk"), output)
}

func pemExport(path, pemType string, bytes []byte) error {
	//write pem out to file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return pem.Encode(file, &pem.Block{Type: pemType, Bytes: bytes})
}
