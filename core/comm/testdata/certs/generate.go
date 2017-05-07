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

// +build ignore

//go:generate -command gencerts go run $GOPATH/src/github.com/hyperledger/fabric/core/comm/testdata/certs/generate.go
//go:generate gencerts -orgs 2 -child-orgs 2 -servers 2 -clients 2

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"
)

//command line flags
var (
	numOrgs        = flag.Int("orgs", 2, "number of unique organizations")
	numChildOrgs   = flag.Int("child-orgs", 2, "number of intermediaries per organization")
	numClientCerts = flag.Int("clients", 1, "number of client certificates per organization")
	numServerCerts = flag.Int("servers", 1, "number of server certificates per organization")
)

//default template for X509 subject
func subjectTemplate() pkix.Name {
	return pkix.Name{
		Country:  []string{"US"},
		Locality: []string{"San Francisco"},
		Province: []string{"California"},
	}
}

//default template for X509 certificates
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

//generate an EC private key (P256 curve)
func genKeyECDSA(name string) (*ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	//write key out to file
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	keyFile, err := os.OpenFile(name+"-key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyFile.Close()
	return priv, nil
}

//generate a signed X509 certficate using ECDSA
func genCertificateECDSA(name string, template, parent *x509.Certificate, pub *ecdsa.PublicKey,
	priv *ecdsa.PrivateKey) (*x509.Certificate, error) {

	//create the x509 public cert
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent, pub, priv)
	if err != nil {
		return nil, err
	}

	//write cert out to file
	certFile, err := os.Create(name + "-cert.pem")
	if err != nil {
		return nil, err
	}
	//pem encode the cert
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	certFile.Close()

	x509Cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	return x509Cert, nil
}

//generate an EC certificate appropriate for use by a TLS server
func genServerCertificateECDSA(name string, signKey *ecdsa.PrivateKey, signCert *x509.Certificate) error {
	fmt.Println(name)
	key, err := genKeyECDSA(name)
	template, err := x509Template()

	if err != nil {
		return err
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	//hardcode to localhost for hostname verification
	subject.CommonName = "localhost"

	template.Subject = subject

	_, err = genCertificateECDSA(name, &template, signCert, &key.PublicKey, signKey)

	if err != nil {
		return err
	}

	return nil
}

//generate an EC certificate appropriate for use by a TLS server
func genClientCertificateECDSA(name string, signKey *ecdsa.PrivateKey, signCert *x509.Certificate) error {
	fmt.Println(name)
	key, err := genKeyECDSA(name)
	template, err := x509Template()

	if err != nil {
		return err
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject

	_, err = genCertificateECDSA(name, &template, signCert, &key.PublicKey, signKey)

	if err != nil {
		return err
	}

	return nil
}

//generate an EC certificate signing(CA) key pair and output as
//PEM-encoded files
func genCertificateAuthorityECDSA(name string) (*ecdsa.PrivateKey, *x509.Certificate, error) {

	key, err := genKeyECDSA(name)
	template, err := x509Template()

	if err != nil {
		return nil, nil, err
	}

	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = []byte{1, 2, 3, 4}

	x509Cert, err := genCertificateECDSA(name, &template, &template, &key.PublicKey, key)

	if err != nil {
		return nil, nil, err
	}
	return key, x509Cert, nil
}

//generate an EC certificate appropriate for use by a TLS server
func genIntermediateCertificateAuthorityECDSA(name string, signKey *ecdsa.PrivateKey,
	signCert *x509.Certificate) (*ecdsa.PrivateKey, *x509.Certificate, error) {

	fmt.Println(name)
	key, err := genKeyECDSA(name)
	template, err := x509Template()

	if err != nil {
		return nil, nil, err
	}

	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = []byte{1, 2, 3, 4}

	x509Cert, err := genCertificateECDSA(name, &template, signCert, &key.PublicKey, signKey)

	if err != nil {
		return nil, nil, err
	}
	return key, x509Cert, nil
}

func main() {

	//parse the command line flags
	flag.Parse()

	fmt.Printf("Generating %d organizations each with %d server(s) and %d client(s)\n",
		*numOrgs, *numServerCerts, *numClientCerts)

	baseOrgName := "Org"
	//generate orgs / CAs
	for i := 1; i <= *numOrgs; i++ {
		signKey, signCert, err := genCertificateAuthorityECDSA(fmt.Sprintf(baseOrgName+"%d", i))
		if err != nil {
			fmt.Printf("error generating CA %s%d : %s\n", baseOrgName, i, err.Error())
		}
		//generate server certificates for the org
		for j := 1; j <= *numServerCerts; j++ {
			err := genServerCertificateECDSA(fmt.Sprintf(baseOrgName+"%d-server%d", i, j), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating server certificate for %s%d-server%d : %s\n",
					baseOrgName, i, j, err.Error())
			}
		}
		//generate client certificates for the org
		for k := 1; k <= *numClientCerts; k++ {
			err := genClientCertificateECDSA(fmt.Sprintf(baseOrgName+"%d-client%d", i, k), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating client certificate for %s%d-client%d : %s\n",
					baseOrgName, i, k, err.Error())
			}
		}
		//generate child orgs (intermediary authorities)
		for m := 1; m <= *numChildOrgs; m++ {
			childSignKey, childSignCert, err := genIntermediateCertificateAuthorityECDSA(
				fmt.Sprintf(baseOrgName+"%d-child%d", i, m), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating CA %s%d-child%d : %s\n",
					baseOrgName, i, m, err.Error())
			}
			//generate server certificates for the child org
			for n := 1; n <= *numServerCerts; n++ {
				err := genServerCertificateECDSA(fmt.Sprintf(baseOrgName+"%d-child%d-server%d", i, m, n),
					childSignKey, childSignCert)
				if err != nil {
					fmt.Printf("error generating server certificate for %s%d-child%d-server%d : %s\n",
						baseOrgName, i, m, n, err.Error())
				}
			}
			//generate client certificates for the child org
			for p := 1; p <= *numClientCerts; p++ {
				err := genClientCertificateECDSA(fmt.Sprintf(baseOrgName+"%d-child%d-client%d", i, m, p),
					childSignKey, childSignCert)
				if err != nil {
					fmt.Printf("error generating server certificate for %s%d-child%d-client%d : %s\n",
						baseOrgName, i, m, p, err.Error())
				}
			}
		}
	}

}
