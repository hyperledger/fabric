/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

// ContainerRouter is a poor abstraction used for building, and running chaincode processes.
// This management probably does not belong in this package, chaincode process lifecycle should
// be driven by what chaincodes are defined, what chaincodes are instantiated, and not driven by
// invocations.  But, the legacy lifecycle makes this very challenging.  Once the legacy lifecycle
// is removed (or perhaps before), this interface should probably go away entirely.
type ContainerRouter interface {
	Build(ccid string) error
	ChaincodeServerInfo(ccid string) (*ccintf.ChaincodeServerInfo, error)
	Start(ccid string, peerConnection *ccintf.PeerConnection) error
	Stop(ccid string) error
	Wait(ccid string) (int, error)
}

// ContainerRuntime is responsible for managing containerized chaincode.
type ContainerRuntime struct {
	ContainerRouter ContainerRouter
	BuildRegistry   *container.BuildRegistry
}

// Build builds the chaincode if necessary and returns ChaincodeServerInfo if
// the chaincode is a server
func (c *ContainerRuntime) Build(ccid string) (*ccintf.ChaincodeServerInfo, error) {
	buildStatus, ok := c.BuildRegistry.BuildStatus(ccid)
	if !ok {
		err := c.ContainerRouter.Build(ccid)
		buildStatus.Notify(err)
	}
	<-buildStatus.Done()

	if err := buildStatus.Err(); err != nil {
		return nil, errors.WithMessage(err, "error building image")
	}

	return c.ContainerRouter.ChaincodeServerInfo(ccid)
}

// Start launches chaincode in a runtime environment.
func (c *ContainerRuntime) Start(ccid string, ccinfo *ccintf.PeerConnection) error {
	chaincodeLogger.Debugf("start container: %s", ccid)

	if err := c.ContainerRouter.Start(ccid, ccinfo); err != nil {
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
