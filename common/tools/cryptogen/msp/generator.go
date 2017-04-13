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

	"github.com/hyperledger/fabric/common/tools/cryptogen/ca"
	"github.com/hyperledger/fabric/common/tools/cryptogen/csp"
)

func GenerateLocalMSP(baseDir, name string, rootCA *ca.CA) error {

	// create folder structure
	err := createFolderStructure(baseDir)
	if err != nil {
		return err
	}

	// generate private key
	priv, _, err := csp.GeneratePrivateKey(filepath.Join(baseDir, "keystore"))
	if err != nil {
		return err
	}

	// get public signing certificate
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return err
	}
	err = rootCA.SignCertificate(filepath.Join(baseDir, "signcerts"),
		name, ecPubKey)
	if err != nil {
		return err
	}

	// write root cert to folders
	err = x509ToFile(filepath.Join(baseDir, "admincerts"), rootCA.Name, rootCA.SignCert)
	if err != nil {
		return err
	}
	err = x509ToFile(filepath.Join(baseDir, "cacerts"), rootCA.Name, rootCA.SignCert)
	if err != nil {
		return err
	}
	return nil

}

func GenerateVerifyingMSP(baseDir string, rootCA *ca.CA) error {

	// create folder structure
	err := createFolderStructure(baseDir)
	if err != nil {
		return err
	}

	// write public cert to appropriate folders
	err = x509ToFile(filepath.Join(baseDir, "admincerts"), rootCA.Name, rootCA.SignCert)
	if err != nil {
		return err
	}
	err = x509ToFile(filepath.Join(baseDir, "cacerts"), rootCA.Name, rootCA.SignCert)
	if err != nil {
		return err
	}
	err = x509ToFile(filepath.Join(baseDir, "signcerts"), rootCA.Name, rootCA.SignCert)
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

func x509ToFile(baseDir, name string, cert *x509.Certificate) error {

	//write cert out to file
	fileName := filepath.Join(baseDir, name+"-cert.pem")
	certFile, err := os.Create(fileName)
	if err != nil {
		return err
	}
	//pem encode the cert
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	certFile.Close()
	if err != nil {
		return err
	}

	return nil

}
