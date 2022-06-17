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

func TestLoadCertificateECDSA(t *testing.T) {
	testDir := t.TempDir()

	// generate private key
	certDir, err := ioutil.TempDir(testDir, "certs")
	if err != nil {
		t.Fatalf("Failed to create certs directory: %s", err)
	}
	priv, err := csp.GeneratePrivateKey(certDir)
	require.NoError(t, err, "Failed to generate signed certificate")

	// create our CA
	caDir := filepath.Join(testDir, "ca")
	rootCA, err := ca.NewCA(
		caDir,
		testCA3Name,
		testCA3Name,
		testCountry,
		testProvince,
		testLocality,
		testOrganizationalUnit,
		testStreetAddress,
		testPostalCode,
	)
	require.NoError(t, err, "Error generating CA")

	cert, err := rootCA.SignCertificate(
		certDir,
		testName3,
		nil,
		nil,
		&priv.PublicKey,
		x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	)
	require.NoError(t, err, "Failed to generate signed certificate")
	// KeyUsage should be x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	require.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		cert.KeyUsage)
	require.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageAny)

	loadedCert, err := ca.LoadCertificateECDSA(certDir)
	require.NoError(t, err)
	require.NotNil(t, loadedCert, "Should load cert")
	require.Equal(t, cert.SerialNumber, loadedCert.SerialNumber, "Should have same serial number")
	require.Equal(t, cert.Subject.CommonName, loadedCert.Subject.CommonName, "Should have same CN")
}

func TestLoadCertificateECDSA_wrongEncoding(t *testing.T) {
	testDir := t.TempDir()

	filename := filepath.Join(testDir, "wrong_encoding.pem")
	err := ioutil.WriteFile(filename, []byte("wrong_encoding"), 0o644) // Wrong encoded cert
	require.NoErrorf(t, err, "failed to create file %s", filename)

	_, err = ca.LoadCertificateECDSA(testDir)
	require.NotNil(t, err)
	require.EqualError(t, err, filename+": wrong PEM encoding")
}

func TestLoadCertificateECDSA_empty_DER_cert(t *testing.T) {
	testDir := t.TempDir()

	filename := filepath.Join(testDir, "empty.pem")
	empty_cert := "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"
	err := ioutil.WriteFile(filename, []byte(empty_cert), 0o644)
	require.NoErrorf(t, err, "failed to create file %s", filename)

	cert, err := ca.LoadCertificateECDSA(testDir)
	require.Nil(t, cert)
	require.NotNil(t, err)
	require.EqualError(t, err, filename+": wrong DER encoding")
}

func TestNewCA(t *testing.T) {
	testDir := t.TempDir()

	caDir := filepath.Join(testDir, "ca")
	rootCA, err := ca.NewCA(
		caDir,
		testCAName,
		testCAName,
		testCountry,
		testProvince,
		testLocality,
		testOrganizationalUnit,
		testStreetAddress,
		testPostalCode,
	)
	require.NoError(t, err, "Error generating CA")
	require.NotNil(t, rootCA, "Failed to return CA")
	require.NotNil(t, rootCA.Signer,
		"rootCA.Signer should not be empty")
	require.IsType(t, &x509.Certificate{}, rootCA.SignCert,
		"rootCA.SignCert should be type x509.Certificate")

	// check to make sure the root public key was stored
	pemFile := filepath.Join(caDir, testCAName+"-cert.pem")
	require.Equal(t, true, checkForFile(pemFile),
		"Expected to find file "+pemFile)

	require.NotEmpty(t, rootCA.SignCert.Subject.Country, "country cannot be empty.")
	require.Equal(t, testCountry, rootCA.SignCert.Subject.Country[0], "Failed to match country")
	require.NotEmpty(t, rootCA.SignCert.Subject.Province, "province cannot be empty.")
	require.Equal(t, testProvince, rootCA.SignCert.Subject.Province[0], "Failed to match province")
	require.NotEmpty(t, rootCA.SignCert.Subject.Locality, "locality cannot be empty.")
	require.Equal(t, testLocality, rootCA.SignCert.Subject.Locality[0], "Failed to match locality")
	require.NotEmpty(t, rootCA.SignCert.Subject.OrganizationalUnit, "organizationalUnit cannot be empty.")
	require.Equal(t, testOrganizationalUnit, rootCA.SignCert.Subject.OrganizationalUnit[0], "Failed to match organizationalUnit")
	require.NotEmpty(t, rootCA.SignCert.Subject.StreetAddress, "streetAddress cannot be empty.")
	require.Equal(t, testStreetAddress, rootCA.SignCert.Subject.StreetAddress[0], "Failed to match streetAddress")
	require.NotEmpty(t, rootCA.SignCert.Subject.PostalCode, "postalCode cannot be empty.")
	require.Equal(t, testPostalCode, rootCA.SignCert.Subject.PostalCode[0], "Failed to match postalCode")
}

func TestGenerateSignCertificate(t *testing.T) {
	testDir := t.TempDir()

	// generate private key
	certDir, err := ioutil.TempDir(testDir, "certs")
	if err != nil {
		t.Fatalf("Failed to create certs directory: %s", err)
	}
	priv, err := csp.GeneratePrivateKey(certDir)
	require.NoError(t, err, "Failed to generate signed certificate")

	// create our CA
	caDir := filepath.Join(testDir, "ca")
	rootCA, err := ca.NewCA(
		caDir,
		testCA2Name,
		testCA2Name,
		testCountry,
		testProvince,
		testLocality,
		testOrganizationalUnit,
		testStreetAddress,
		testPostalCode,
	)
	require.NoError(t, err, "Error generating CA")

	cert, err := rootCA.SignCertificate(
		certDir,
		testName,
		nil,
		nil,
		&priv.PublicKey,
		x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	)
	require.NoError(t, err, "Failed to generate signed certificate")
	// KeyUsage should be x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	require.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment,
		cert.KeyUsage)
	require.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageAny)

	cert, err = rootCA.SignCertificate(
		certDir,
		testName,
		nil,
		nil,
		&priv.PublicKey,
		x509.KeyUsageDigitalSignature,
		[]x509.ExtKeyUsage{},
	)
	require.NoError(t, err, "Failed to generate signed certificate")
	require.Equal(t, 0, len(cert.ExtKeyUsage))

	// make sure ous are correctly set
	ous := []string{"TestOU", "PeerOU"}
	cert, err = rootCA.SignCertificate(certDir, testName, ous, nil, &priv.PublicKey,
		x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	require.NoError(t, err)
	require.Contains(t, cert.Subject.OrganizationalUnit, ous[0])
	require.Contains(t, cert.Subject.OrganizationalUnit, ous[1])

	// make sure sans are correctly set
	sans := []string{testName2, testName3, testIP}
	cert, err = rootCA.SignCertificate(certDir, testName, nil, sans, &priv.PublicKey,
		x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{})
	require.NoError(t, err)
	require.Contains(t, cert.DNSNames, testName2)
	require.Contains(t, cert.DNSNames, testName3)
	require.Contains(t, cert.IPAddresses, net.ParseIP(testIP).To4())
	require.Equal(t, len(cert.DNSNames), 2)

	// check to make sure the signed public key was stored
	pemFile := filepath.Join(certDir, testName+"-cert.pem")
	require.Equal(t, true, checkForFile(pemFile),
		"Expected to find file "+pemFile)

	_, err = rootCA.SignCertificate(certDir, "empty/CA", nil, nil, &priv.PublicKey,
		x509.KeyUsageKeyEncipherment, []x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	require.Error(t, err, "Bad name should fail")

	// use an empty CA to test error path
	badCA := &ca.CA{
		Name:     "badCA",
		SignCert: &x509.Certificate{},
	}
	_, err = badCA.SignCertificate(certDir, testName, nil, nil, &ecdsa.PublicKey{},
		x509.KeyUsageKeyEncipherment, []x509.ExtKeyUsage{x509.ExtKeyUsageAny})
	require.Error(t, err, "Empty CA should not be able to sign")
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
