/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
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
	Build(ccid string) error
	Start(ccid string, peerConnection *ccintf.PeerConnection) error
	Stop(ccid string) error
	Wait(ccid string) (int, error)
}

// ContainerRuntime is responsible for managing containerized chaincode.
type ContainerRuntime struct {
	CertGenerator   CertGenerator
	ContainerRouter ContainerRouter
	CACert          []byte
	PeerAddress     string
}

// Start launches chaincode in a runtime environment.
func (c *ContainerRuntime) Start(ccid string) error {
	var tlsConfig *ccintf.TLSConfig
	if c.CertGenerator != nil {
		certKeyPair, err := c.CertGenerator.Generate(string(ccid))
		if err != nil {
			return errors.WithMessagef(err, "failed to generate TLS certificates for %s", ccid)
		}

		tlsConfig = &ccintf.TLSConfig{
			ClientCert: certKeyPair.Cert,
			ClientKey:  certKeyPair.Key,
			RootCert:   c.CACert,
		}
	}

	if err := c.ContainerRouter.Build(ccid); err != nil {
		return errors.WithMessage(err, "error building image")
	}

	chaincodeLogger.Debugf("start container: %s", ccid)

	if err := c.ContainerRouter.Start(
		ccid,
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
func (c *ContainerRuntime) Stop(ccid string) error {
	if err := c.ContainerRouter.Stop(ccid); err != nil {
		return errors.WithMessage(err, "error stopping container")
	}

	return nil
}

// Wait waits for the container runtime to terminate.
func (c *ContainerRuntime) Wait(ccid string) (int, error) {
	return c.ContainerRouter.Wait(ccid)
}
