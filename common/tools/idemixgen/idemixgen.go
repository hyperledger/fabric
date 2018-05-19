/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

// idemixgen is a command line tool that generates the CA's keys and
// generates MSP configs for siging and for verification
// This tool can be used to setup the peers and CA to support
// the Identity Mixer MSP

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"crypto/elliptic"

	"crypto/ecdsa"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/tools/idemixgen/idemixca"
	"github.com/hyperledger/fabric/common/tools/idemixgen/metadata"
	"github.com/hyperledger/fabric/idemix"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	IdemixDirIssuer             = "ca"
	IdemixConfigIssuerSecretKey = "IssuerSecretKey"
	IdemixConfigRevocationKey   = "RevocationKey"
)

// command line flags
var (
	app = kingpin.New("idemixgen", "Utility for generating key material to be used with the Identity Mixer MSP in Hyperledger Fabric")

	outputDir = app.Flag("output", "The output directory in which to place artifacts").Default("idemix-config").String()

	genIssuerKey            = app.Command("ca-keygen", "Generate CA key material")
	genSignerConfig         = app.Command("signerconfig", "Generate a default signer for this Idemix MSP")
	genCredOU               = genSignerConfig.Flag("org-unit", "The Organizational Unit of the default signer").Short('u').String()
	genCredIsAdmin          = genSignerConfig.Flag("admin", "Make the default signer admin").Short('a').Bool()
	genCredEnrollmentId     = genSignerConfig.Flag("enrollmentId", "The enrollment id of the default signer").Short('e').String()
	genCredRevocationHandle = genSignerConfig.Flag("revocationHandle", "The handle used to revoke this signer").Short('r').Int()

	version = app.Command("version", "Show version information")
)

func main() {
	app.HelpFlag.Short('h')

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {

	case genIssuerKey.FullCommand():
		isk, ipk, err := idemixca.GenerateIssuerKey()
		handleError(err)

		revocationKey, err := idemix.GenerateLongTermRevocationKey()
		handleError(err)
		revocationKeyBytes, err := x509.MarshalECPrivateKey(revocationKey)
		handleError(err)
		revocationPkBytes := elliptic.Marshal(elliptic.P384(), revocationKey.X, revocationKey.Y)

		// Prevent overwriting the existing key
		path := filepath.Join(*outputDir, IdemixDirIssuer)
		checkDirectoryNotExists(path, fmt.Sprintf("Directory %s already exists", path))

		path = filepath.Join(*outputDir, msp.IdemixConfigDirMsp)
		checkDirectoryNotExists(path, fmt.Sprintf("Directory %s already exists", path))

		// write private and public keys to the file
		handleError(os.MkdirAll(filepath.Join(*outputDir, IdemixDirIssuer), 0770))
		handleError(os.MkdirAll(filepath.Join(*outputDir, msp.IdemixConfigDirMsp), 0770))
		writeFile(filepath.Join(*outputDir, IdemixDirIssuer, IdemixConfigIssuerSecretKey), isk)
		writeFile(filepath.Join(*outputDir, IdemixDirIssuer, IdemixConfigRevocationKey), revocationKeyBytes)
		writeFile(filepath.Join(*outputDir, IdemixDirIssuer, msp.IdemixConfigFileIssuerPublicKey), ipk)
		writeFile(filepath.Join(*outputDir, msp.IdemixConfigDirMsp, msp.IdemixConfigFileRevocationPublicKey), revocationPkBytes)
		writeFile(filepath.Join(*outputDir, msp.IdemixConfigDirMsp, msp.IdemixConfigFileIssuerPublicKey), ipk)

	case genSignerConfig.FullCommand():
		config, err := idemixca.GenerateSignerConfig(*genCredIsAdmin, *genCredOU, *genCredEnrollmentId, *genCredRevocationHandle, readIssuerKey(), readRevocationKey())
		handleError(err)

		path := filepath.Join(*outputDir, msp.IdemixConfigDirUser)
		checkDirectoryNotExists(path, fmt.Sprintf("This MSP config already contains a directory \"%s\"", path))

		// Write config to file
		handleError(os.Mkdir(filepath.Join(*outputDir, msp.IdemixConfigDirUser), 0770))
		writeFile(filepath.Join(*outputDir, msp.IdemixConfigDirUser, msp.IdemixConfigFileSigner), config)

	case version.FullCommand():
		printVersion()
	}
}

func printVersion() {
	fmt.Println(metadata.GetVersionInfo())
}

// writeFile writes bytes to a file and panics in case of an error
func writeFile(path string, contents []byte) {
	handleError(ioutil.WriteFile(path, contents, 0640))
}

// readIssuerKey reads the issuer key from the current directory
func readIssuerKey() *idemix.IssuerKey {
	path := filepath.Join(*outputDir, IdemixDirIssuer, IdemixConfigIssuerSecretKey)
	isk, err := ioutil.ReadFile(path)
	if err != nil {
		handleError(errors.Wrapf(err, "failed to open issuer secret key file: %s", path))
	}
	path = filepath.Join(*outputDir, IdemixDirIssuer, msp.IdemixConfigFileIssuerPublicKey)
	ipkBytes, err := ioutil.ReadFile(path)
	if err != nil {
		handleError(errors.Wrapf(err, "failed to open issuer public key file: %s", path))
	}
	ipk := &idemix.IssuerPublicKey{}
	handleError(proto.Unmarshal(ipkBytes, ipk))
	key := &idemix.IssuerKey{isk, ipk}

	return key
}

func readRevocationKey() *ecdsa.PrivateKey {
	path := filepath.Join(*outputDir, IdemixDirIssuer, IdemixConfigRevocationKey)
	keyBytes, err := ioutil.ReadFile(path)
	if err != nil {
		handleError(errors.Wrapf(err, "failed to open revocation secret key file: %s", path))
	}
	key, err := x509.ParseECPrivateKey(keyBytes)
	handleError(err)

	return key
}

// checkDirectoryNotExists checks whether a directory with the given path already exists and exits if this is the case
func checkDirectoryNotExists(path string, errorMessage string) {
	_, err := os.Stat(path)
	if err == nil {
		handleError(errors.New(errorMessage))
	}
}

func handleError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
