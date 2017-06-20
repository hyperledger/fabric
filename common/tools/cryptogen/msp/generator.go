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
	"io"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/tools/cryptogen/ca"
	"github.com/hyperledger/fabric/common/tools/cryptogen/csp"
)

func GenerateLocalMSP(baseDir, name string, sans []string, rootCA *ca.CA) error {

	// create folder structure
	mspDir := filepath.Join(baseDir, "msp")
	tlsDir := filepath.Join(baseDir, "tls")

	err := createFolderStructure(mspDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(tlsDir, 0755)
	if err != nil {
		return err
	}

	// get keystore path
	keystore := filepath.Join(mspDir, "keystore")

	// generate private key
	priv, _, err := csp.GeneratePrivateKey(keystore)
	if err != nil {
		return err
	}

	// get public signing certificate
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return err
	}

	cert, err := rootCA.SignCertificate(filepath.Join(mspDir, "signcerts"), name, sans, ecPubKey)
	if err != nil {
		return err
	}

	// write artifacts to MSP folders

	// the CA certificate goes into cacerts
	folders := []string{"cacerts"}
	for _, folder := range folders {
		err = x509Export(filepath.Join(mspDir, folder, x509Filename(rootCA.Name)), rootCA.SignCert)
		if err != nil {
			return err
		}
	}

	// the signing identity goes into admincerts.
	// This means that the signing identity
	// of this MSP is also an admin of this MSP
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	folders = []string{"admincerts"}
	for _, folder := range folders {
		err = x509Export(filepath.Join(mspDir, folder, x509Filename(rootCA.Name)), cert)
		if err != nil {
			return err
		}
	}

	// write artifacts to TLS folder
	err = x509Export(filepath.Join(tlsDir, "ca.crt"), rootCA.SignCert)
	if err != nil {
		return err
	}

	err = x509Export(filepath.Join(tlsDir, "server.crt"), cert)
	if err != nil {
		return err
	}

	err = keyExport(keystore, filepath.Join(tlsDir, "server.key"), priv)
	if err != nil {
		return err
	}

	return nil
}

func GenerateVerifyingMSP(baseDir string, rootCA *ca.CA) error {

	// create folder structure
	err := createFolderStructure(baseDir)
	if err == nil {
		// write MSP cert to appropriate folders
		folders := []string{"cacerts", "signcerts"}
		for _, folder := range folders {
			err = x509Export(filepath.Join(baseDir, folder, x509Filename(rootCA.Name)), rootCA.SignCert)
			if err != nil {
				return err
			}
		}
	}

	// create a throwaway cert to act as an admin cert
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	bcsp := factory.GetDefault()
	priv, err := bcsp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: true})
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return err
	}
	_, err = rootCA.SignCertificate(filepath.Join(baseDir, "admincerts"), rootCA.Name, []string{""}, ecPubKey)
	if err != nil {
		return err
	}

	return nil
}

func createFolderStructure(rootDir string) error {

	// create admincerts, cacerts, keystore and signcerts folders
	folders := []string{
		filepath.Join(rootDir, "admincerts"),
		filepath.Join(rootDir, "cacerts"),
		filepath.Join(rootDir, "keystore"),
		filepath.Join(rootDir, "signcerts"),
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func keyExport(keystore, output string, key bccsp.Key) error {
	id := hex.EncodeToString(key.SKI())

	return copyFile(filepath.Join(keystore, id+"_sk"), output)
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
