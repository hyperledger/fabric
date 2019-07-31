/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

// CertGenerator generates client certificates for chaincode.
type CertGenerator interface {
	// Generate returns a certificate and private key and associates
	// the hash of the certificates with the given chaincode name
	Generate(ccName string) (*accesscontrol.CertAndPrivKeyPair, error)
}

// ContainerRouter is a poor abstraction used for building, and running chaincode processes.
// This management probably does not belong in this package, chaincode process lifecycle should
// be driven by what chaincodes are defined, what chaincodes are instantiated, and not driven by
// invocations.  But, the legacy lifecycle makes this very challenging.  Once the legacy lifecycle
// is removed (or perhaps before), this interface should probably go away entirely.
type ContainerRouter interface {
	Build(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) error
	Start(ccid ccintf.CCID, peerConnection *ccintf.PeerConnection) error
	Stop(ccid ccintf.CCID) error
	Wait(ccid ccintf.CCID) (int, error)
}

// ContainerRuntime is responsible for managing containerized chaincode.
type ContainerRuntime struct {
	CertGenerator   CertGenerator
	ContainerRouter ContainerRouter
	CACert          []byte
	PeerAddress     string
}

// Start launches chaincode in a runtime environment.
func (c *ContainerRuntime) Start(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) error {
	packageID := ccci.PackageID.String()

	var tlsConfig *ccintf.TLSConfig
	if c.CertGenerator != nil {
		certKeyPair, err := c.CertGenerator.Generate(packageID)
		if err != nil {
			return errors.WithMessagef(err, "failed to generate TLS certificates for %s", packageID)
		}

		tlsConfig = &ccintf.TLSConfig{
			ClientCert: certKeyPair.Cert,
			ClientKey:  certKeyPair.Key,
			RootCert:   c.CACert,
		}
	}

	if err := c.ContainerRouter.Build(ccci, codePackage); err != nil {
		return errors.WithMessage(err, "error building image")
	}

	chaincodeLogger.Debugf("start container: %s", packageID)

	if err := c.ContainerRouter.Start(
		ccintf.New(ccci.PackageID),
		&ccintf.PeerConnection{
			Address:   c.PeerAddress,
			TLSConfig: tlsConfig,
		},
	); err != nil {
		return errors.WithMessage(err, "error starting container")
	}

	return nil
}

// Stop terminates chaincode and its container runtime environment.
func (c *ContainerRuntime) Stop(ccci *ccprovider.ChaincodeContainerInfo) error {
	if err := c.ContainerRouter.Stop(ccintf.New(ccci.PackageID)); err != nil {
		return errors.WithMessage(err, "error stopping container")
	}

	return nil
}

// Wait waits for the container runtime to terminate.
func (c *ContainerRuntime) Wait(ccci *ccprovider.ChaincodeContainerInfo) (int, error) {
	return c.ContainerRouter.Wait(ccintf.New(ccci.PackageID))
}
