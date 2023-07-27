/*
Copyright Ahmed Al Salih. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bdls

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/BDLS-bft/bdls"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/pkg/errors"
)

// Chain represents a BDLS chain.
type Chain struct {
	Config  *bdls.Config
	logger  *flogging.FabricLogger
	support consensus.ConsenterSupport

	opts Options
}

type Options struct {
	// BlockMetadata and Consenters should only be modified while under lock
	// of raftMetadataLock
	BlockMetadata *etcdraft.BlockMetadata
	Consenters    map[uint64]*etcdraft.Consenter
}

// Order accepts a message which has been processed at a given configSeq.
// If the configSeq advances, it is the responsibility of the consenter
// to revalidate and potentially discard the message
// The consenter may return an error, indicating the message was not accepted
func (c *Chain) Order(env *cb.Envelope, configSeq uint64) error {
	//TODO
	return nil
}

// Configure accepts a message which reconfigures the channel and will
// trigger an update to the configSeq if committed.  The configuration must have
// been triggered by a ConfigUpdate message. If the config sequence advances,
// it is the responsibility of the consenter to recompute the resulting config,
// discarding the message if the reconfiguration is no longer valid.
// The consenter may return an error, indicating the message was not accepted
func (c *Chain) Configure(config *cb.Envelope, configSeq uint64) error {
	//TODO
	return nil
}

// WaitReady blocks waiting for consenter to be ready for accepting new messages.
// This is useful when consenter needs to temporarily block ingress messages so
// that in-flight messages can be consumed. It could return error if consenter is
// in erroneous states. If this blocking behavior is not desired, consenter could
// simply return nil.
func (c *Chain) WaitReady() error {
	//TODO
	return nil
}

// Errored returns a channel which will close when an error has occurred.
// This is especially useful for the Deliver client, who must terminate waiting
// clients when the consenter is not up to date.
func (c *Chain) Errored() <-chan struct{} {
	//TODO
	return nil
}

// Start should allocate whatever resources are needed for staying up to date with the chain.
// Typically, this involves creating a thread which reads from the ordering source, passes those
// messages to a block cutter, and writes the resulting blocks to the ledger.
func (c *Chain) Start() {
	// create configuration
	config := new(bdls.Config)
	config.Epoch = time.Now()
	config.CurrentHeight = c.support.Height()
	config.StateCompare = func(a bdls.State, b bdls.State) int { return bytes.Compare(a, b) }
	config.StateValidate = func(bdls.State) bool { return true }
	//config.PrivateKey =
	//config.Participants =

	//c.opts.Consenters
}

// NewChain constructs a chain object.
func NewChain() (*Chain, error) {

	c := &Chain{}
	return c, nil

}

// Halt frees the resources which were allocated for this Chain.
func (c *Chain) Halt() {
	//TODO
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// publicKeyFromCertificate returns the public key of the given ASN1 DER certificate.
func publicKeyFromCertificate(der []byte) ([]byte, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return x509.MarshalPKIXPublicKey(cert.PublicKey)
}
