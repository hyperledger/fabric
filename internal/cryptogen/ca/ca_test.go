/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package ca_test

import (
	"crypto/ecdsa"
	"crypto/x509"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/internal/cryptogen/ca"
	"github.com/hyperledger/fabric/internal/cryptogen/csp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCAName             = "root0"
	testCA2Name            = "root1"
	testCA3Name            = "root2"
	testName               = "cert0"
	testName2              = "cert1"
	testName3              = "cert2"
	testIP                 = "172.16.10.31"
	testCountry            = "US"
	testProvince           = "California"
	testLocality           = "San Francisco"
	testOrganizationalUnit = "Hyperledger Fabric"
	testStreetAddress      = "testStreetAddress"
	testPostalCode         = "123456"
)

var testDir = filepath.Join(os.TempDir(), "ca-test")

func TestLoadCertificateECDSA(t *testing.T) {
	caDir := filepath.Join(testDir, "ca")
	certDir := filepath.Join(testDir, "certs")
	// generate private key
	priv, _, err := csp.GeneratePrivateKey(certDir)
	assert.NoError(t, err, "Failed to generate signed certificate")

	// get EC public key
	ecPubKey, err := csp.GetECPublicKey(priv)
	assert.NoError(t, err, "Failed to generate signed certificate")
	assert.NotNil(t, ecPubKey, "Failed to generate signed certificate")

	// create our CA
	rootCA, err := ca.NewCA(caDir, testCA3Name, testCA3Name, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")

	cert, err := rootCA.SignCertificate(certDir, testName3, nil, nil, ecPubKey,
		x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	assert.NoError(t, err, "Failed to generate signed certificate")
	// KeyUsage should be x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		cert.KeyUsage)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageAny)

	loadedCert, err := ca.LoadCertificateECDSA(certDir)
	assert.NotNil(t, loadedCert, "Should load cert")
	assert.Equal(t, cert.SerialNumber, loadedCert.SerialNumber, "Should have same serial number")
	assert.Equal(t, cert.Subject.CommonName, loadedCert.Subject.CommonName, "Should have same CN")
	cleanup(testDir)
}

func TestLoadCertificateECDSA_wrongEncoding(t *testing.T) {
	dir, err := ioutil.TempDir("", "wrongEncoding")
	require.NoError(t, err, "failed to create test directory")
	defer cleanup(dir)

	filename := filepath.Join(dir, "wrong_encoding.pem")
	err = ioutil.WriteFile(filename, []byte("wrong_encoding"), 0644) // Wrong encoded cert
	require.NoErrorf(t, err, "failed to create file %s", filename)

	_, err = ca.LoadCertificateECDSA(dir)
	assert.NotNil(t, err)
	assert.EqualError(t, err, filename+": wrong PEM encoding")
}

func TestLoadCertificateECDSA_empty_DER_cert(t *testing.T) {
	dir, err := ioutil.TempDir("", "empty_cert")
	require.NoError(t, err, "failed to create test directory")
	defer cleanup(dir)

	filename := filepath.Join(dir, "empty_cert.pem")
	empty_cert := "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"
	err = ioutil.WriteFile(filename, []byte(empty_cert), 0644)
	require.NoErrorf(t, err, "failed to create file %s", filename)

	cert, err := ca.LoadCertificateECDSA(dir)
	assert.Nil(t, cert)
	assert.NotNil(t, err)
	assert.EqualError(t, err, filename+": wrong DER encoding")
}

func TestNewCA(t *testing.T) {

	caDir := filepath.Join(testDir, "ca")
	rootCA, err := ca.NewCA(caDir, testCAName, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")
	assert.NotNil(t, rootCA, "Failed to return CA")
	assert.NotNil(t, rootCA.Signer,
		"rootCA.Signer should not be empty")
	assert.IsType(t, &x509.Certificate{}, rootCA.SignCert,
		"rootCA.SignCert should be type x509.Certificate")

	// check to make sure the root public key was stored
	pemFile := filepath.Join(caDir, testCAName+"-cert.pem")
	assert.Equal(t, true, checkForFile(pemFile),
		"Expected to find file "+pemFile)

	assert.NotEmpty(t, rootCA.SignCert.Subject.Country, "country cannot be empty.")
	assert.Equal(t, testCountry, rootCA.SignCert.Subject.Country[0], "Failed to match country")
	assert.NotEmpty(t, rootCA.SignCert.Subject.Province, "province cannot be empty.")
	assert.Equal(t, testProvince, rootCA.SignCert.Subject.Province[0], "Failed to match province")
	assert.NotEmpty(t, rootCA.SignCert.Subject.Locality, "locality cannot be empty.")
	assert.Equal(t, testLocality, rootCA.SignCert.Subject.Locality[0], "Failed to match locality")
	assert.NotEmpty(t, rootCA.SignCert.Subject.OrganizationalUnit, "organizationalUnit cannot be empty.")
	assert.Equal(t, testOrganizationalUnit, rootCA.SignCert.Subject.OrganizationalUnit[0], "Failed to match organizationalUnit")
	assert.NotEmpty(t, rootCA.SignCert.Subject.StreetAddress, "streetAddress cannot be empty.")
	assert.Equal(t, testStreetAddress, rootCA.SignCert.Subject.StreetAddress[0], "Failed to match streetAddress")
	assert.NotEmpty(t, rootCA.SignCert.Subject.PostalCode, "postalCode cannot be empty.")
	assert.Equal(t, testPostalCode, rootCA.SignCert.Subject.PostalCode[0], "Failed to match postalCode")

	cleanup(testDir)

}

func TestGenerateSignCertificate(t *testing.T) {

	caDir := filepath.Join(testDir, "ca")
	certDir := filepath.Join(testDir, "certs")
	// generate private key
	priv, _, err := csp.GeneratePrivateKey(certDir)
	assert.NoError(t, err, "Failed to generate signed certificate")

	// get EC public key
	ecPubKey, err := csp.GetECPublicKey(priv)
	assert.NoError(t, err, "Failed to generate signed certificate")
	assert.NotNil(t, ecPubKey, "Failed to generate signed certificate")

	// create our CA
	rootCA, err := ca.NewCA(caDir, testCA2Name, testCA2Name, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")

	cert, err := rootCA.SignCertificate(certDir, testName, nil, nil, ecPubKey,
		x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	assert.NoError(t, err, "Failed to generate signed certificate")
	// KeyUsage should be x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		cert.KeyUsage)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageAny)

	cert, err = rootCA.SignCertificate(certDir, testName, nil, nil, ecPubKey,
		x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	assert.NoError(t, err, "Failed to generate signed certificate")
	assert.Equal(t, 0, len(cert.ExtKeyUsage))

	// make sure ous are correctly set
	ous := []string{"TestOU", "PeerOU"}
	cert, err = rootCA.SignCertificate(certDir, testName, ous, nil, ecPubKey,
		x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	assert.Contains(t, cert.Subject.OrganizationalUnit, ous[0])
	assert.Contains(t, cert.Subject.OrganizationalUnit, ous[1])

	// make sure sans are correctly set
	sans := []string{testName2, testIP}
	cert, err = rootCA.SignCertificate(certDir, testName, nil, sans, ecPubKey,
		x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	assert.Contains(t, cert.DNSNames, testName2)
	assert.Contains(t, cert.IPAddresses, net.ParseIP(testIP).To4())

	// check to make sure the signed public key was stored
	pemFile := filepath.Join(certDir, testName+"-cert.pem")
	assert.Equal(t, true, checkForFile(pemFile),
		"Expected to find file "+pemFile)

	_, err = rootCA.SignCertificate(certDir, "empty/CA", nil, nil, ecPubKey,
		x509.KeyUsageKeyEncipherment, []x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	assert.Error(t, err, "Bad name should fail")

	// use an empty CA to test error path
	badCA := &ca.CA{
		Name:     "badCA",
		SignCert: &x509.Certificate{},
	}
	_, err = badCA.SignCertificate(certDir, testName, nil, nil, &ecdsa.PublicKey{},
		x509.KeyUsageKeyEncipherment, []x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	assert.Error(t, err, "Empty CA should not be able to sign")
	cleanup(testDir)

}

func cleanup(dir string) {
	os.RemoveAll(dir)
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
