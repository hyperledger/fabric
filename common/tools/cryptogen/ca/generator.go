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
package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"path/filepath"

	"github.com/hyperledger/fabric/common/tools/cryptogen/csp"
)

type CA struct {
	Name string
	//SignKey  *ecdsa.PrivateKey
	Signer   crypto.Signer
	SignCert *x509.Certificate
}

// NewCA creates an instance of CA and saves the signing key pair in
// baseDir/name
func NewCA(baseDir, name string) (*CA, error) {

	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		return nil, err
	}

	//key, err := genKeyECDSA(caDir, name)
	priv, signer, err := csp.GeneratePrivateKey(baseDir)
	// get public signing certificate
	ecPubKey, err := csp.GetECPublicKey(priv)
	if err != nil {
		return nil, err
	}

	template, err := x509Template()

	if err != nil {
		return nil, err
	}

	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny, x509.ExtKeyUsageServerAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = priv.SKI()

	x509Cert, err := genCertificateECDSA(baseDir, name, &template, &template,
		ecPubKey, signer)

	if err != nil {
		return nil, err
	}

	ca := &CA{
		Name:     name,
		Signer:   signer,
		SignCert: x509Cert,
	}

	return ca, nil
}

// SignCertificate creates a signed certificate based on a built-in template
// and saves it in baseDir/name
func (ca *CA) SignCertificate(baseDir, name string, pub *ecdsa.PublicKey) error {

	template, err := x509Template()

	if err != nil {
		return err
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.CommonName = name

	template.Subject = subject

	_, err = genCertificateECDSA(baseDir, name, &template, ca.SignCert,
		pub, ca.Signer)

	if err != nil {
		return err
	}

	return nil
}

// default template for X509 subject
func subjectTemplate() pkix.Name {
	return pkix.Name{
		Country:  []string{"US"},
		Locality: []string{"San Francisco"},
		Province: []string{"California"},
	}
}

// default template for X509 certificates
func x509Template() (x509.Certificate, error) {

	//generate a serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return x509.Certificate{}, err
	}

	now := time.Now()
	//basic template to use
	x509 := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             now,
		NotAfter:              now.Add(3650 * 24 * time.Hour), //~ten years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	return x509, nil

}

// generate a signed X509 certficate using ECDSA
func genCertificateECDSA(baseDir, name string, template, parent *x509.Certificate, pub *ecdsa.PublicKey,
	priv interface{}) (*x509.Certificate, error) {

	//create the x509 public cert
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent, pub, priv)
	if err != nil {
		return nil, err
	}

	//write cert out to file
	fileName := filepath.Join(baseDir, name+"-cert.pem")
	certFile, err := os.Create(fileName)
	if err != nil {
		return nil, err
	}
	//pem encode the cert
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	certFile.Close()
	if err != nil {
		return nil, err
	}

	x509Cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	return x509Cert, nil
}
