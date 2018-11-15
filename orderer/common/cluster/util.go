/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"encoding/hex"
	"encoding/pem"
	"reflect"
	"sync/atomic"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// ConnByCertMap maps certificates represented as strings
// to gRPC connections
type ConnByCertMap map[string]*grpc.ClientConn

// Lookup looks up a certificate and returns the connection that was mapped
// to the certificate, and whether it was found or not
func (cbc ConnByCertMap) Lookup(cert []byte) (*grpc.ClientConn, bool) {
	conn, ok := cbc[string(cert)]
	return conn, ok
}

// Put associates the given connection to the certificate
func (cbc ConnByCertMap) Put(cert []byte, conn *grpc.ClientConn) {
	cbc[string(cert)] = conn
}

// Remove removes the connection that is associated to the given certificate
func (cbc ConnByCertMap) Remove(cert []byte) {
	delete(cbc, string(cert))
}

// MemberMapping defines NetworkMembers by their ID
type MemberMapping map[uint64]*Stub

// Put inserts the given stub to the MemberMapping
func (mp MemberMapping) Put(stub *Stub) {
	mp[stub.ID] = stub
}

// ByID retrieves the Stub with the given ID from the MemberMapping
func (mp MemberMapping) ByID(ID uint64) *Stub {
	return mp[ID]
}

// LookupByClientCert retrieves a Stub with the given client certificate
func (mp MemberMapping) LookupByClientCert(cert []byte) *Stub {
	for _, stub := range mp {
		if bytes.Equal(stub.ClientTLSCert, cert) {
			return stub
		}
	}
	return nil
}

// ServerCertificates returns a set of the server certificates
// represented as strings
func (mp MemberMapping) ServerCertificates() StringSet {
	res := make(StringSet)
	for _, member := range mp {
		res[string(member.ServerTLSCert)] = struct{}{}
	}
	return res
}

// StringSet is a set of strings
type StringSet map[string]struct{}

// union adds the elements of the given set to the StringSet
func (ss StringSet) union(set StringSet) {
	for k := range set {
		ss[k] = struct{}{}
	}
}

// subtract removes all elements in the given set from the StringSet
func (ss StringSet) subtract(set StringSet) {
	for k := range set {
		delete(ss, k)
	}
}

// PredicateDialer creates gRPC connections
// that are only established if the given predicate
// is fulfilled
type PredicateDialer struct {
	Config atomic.Value
}

// NewTLSPinningDialer creates a new PredicateDialer
func NewTLSPinningDialer(config comm.ClientConfig) *PredicateDialer {
	d := &PredicateDialer{}
	d.SetConfig(config)
	return d
}

// ClientConfig returns the comm.ClientConfig, or an error
// if they cannot be extracted.
func (dialer *PredicateDialer) ClientConfig() (comm.ClientConfig, error) {
	val := dialer.Config.Load()
	if val == nil {
		return comm.ClientConfig{}, errors.New("client config not initialized")
	}
	cc, isClientConfig := val.(comm.ClientConfig)
	if !isClientConfig {
		err := errors.Errorf("value stored is %v, not comm.ClientConfig",
			reflect.TypeOf(val))
		return comm.ClientConfig{}, err
	}
	if cc.SecOpts == nil {
		return comm.ClientConfig{}, errors.New("SecOpts is nil")
	}
	// Copy by value the secure options
	secOpts := *cc.SecOpts
	return comm.ClientConfig{
		AsyncConnect: cc.AsyncConnect,
		Timeout:      cc.Timeout,
		SecOpts:      &secOpts,
		KaOpts:       cc.KaOpts,
	}, nil
}

// SetConfig sets the configuration of the PredicateDialer
func (dialer *PredicateDialer) SetConfig(config comm.ClientConfig) {
	configCopy := comm.ClientConfig{
		AsyncConnect: config.AsyncConnect,
		Timeout:      config.Timeout,
		SecOpts:      &comm.SecureOptions{},
		KaOpts:       &comm.KeepaliveOptions{},
	}
	// Explicitly copy configuration
	if config.SecOpts != nil {
		*configCopy.SecOpts = *config.SecOpts
	}
	if config.KaOpts != nil {
		*configCopy.KaOpts = *config.KaOpts
	} else {
		configCopy.KaOpts = nil
	}

	dialer.Config.Store(configCopy)
}

// Dial creates a new gRPC connection that can only be established, if the remote node's
// certificate chain satisfy verifyFunc
func (dialer *PredicateDialer) Dial(address string, verifyFunc RemoteVerifier) (*grpc.ClientConn, error) {
	cfg := dialer.Config.Load().(comm.ClientConfig)
	cfg.SecOpts.VerifyCertificate = verifyFunc
	client, err := comm.NewGRPCClient(cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return client.NewConnection(address, "")
}

// DERtoPEM returns a PEM representation of the DER
// encoded certificate
func DERtoPEM(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
}

// StandardDialer wraps a PredicateDialer
// to a standard cluster.Dialer that passes in a nil verify function
type StandardDialer struct {
	Dialer *PredicateDialer
}

// Dial dials to the given address
func (bdp *StandardDialer) Dial(address string) (*grpc.ClientConn, error) {
	return bdp.Dialer.Dial(address, nil)
}

//go:generate mockery -dir . -name BlockVerifier -case underscore -output ./mocks/

// BlockVerifier verifies block signatures.
type BlockVerifier interface {
	// VerifyBlockSignature verifies a signature of a block.
	// It has an optional argument of a configuration envelope
	// which would make the block verification to use validation rules
	// based on the given configuration in the ConfigEnvelope.
	// If the config envelope passed is nil, then the validation rules used
	// are the ones that were applied at commit of previous blocks.
	VerifyBlockSignature(sd []*common.SignedData, config *common.ConfigEnvelope) error
}

// BlockSequenceVerifier verifies that the given consecutive sequence
// of blocks is valid.
type BlockSequenceVerifier func([]*common.Block) error

// Dialer creates a gRPC connection to a remote address
type Dialer interface {
	Dial(address string) (*grpc.ClientConn, error)
}

// VerifyBlocks verifies the given consecutive sequence of blocks is valid,
// and returns nil if it's valid, else an error.
func VerifyBlocks(blockBuff []*common.Block, signatureVerifier BlockVerifier) error {
	if len(blockBuff) == 0 {
		return errors.New("buffer is empty")
	}
	// First, we verify that the block hash in every block is:
	// Equal to the hash in the header
	// Equal to the previous hash in the succeeding block
	for i := range blockBuff {
		if err := VerifyBlockHash(i, blockBuff); err != nil {
			return err
		}
	}

	var config *common.ConfigEnvelope
	// Verify all configuration blocks that are found inside the block batch,
	// with the configuration that was committed (nil) or with one that is picked up
	// during iteration over the block batch.
	for _, block := range blockBuff {
		configFromBlock, err := ConfigFromBlock(block)
		if err == errNotAConfig {
			continue
		}
		if err != nil {
			return err
		}
		// The block is a configuration block, so verify it
		if err := VerifyBlockSignature(block, signatureVerifier, config); err != nil {
			return err
		}
		config = configFromBlock
	}

	// Verify the last block's signature
	lastBlock := blockBuff[len(blockBuff)-1]
	return VerifyBlockSignature(lastBlock, signatureVerifier, config)
}

var errNotAConfig = errors.New("not a config block")

// ConfigFromBlock returns a ConfigEnvelope if exists, or a *NotAConfigBlock error.
// It may also return some other error in case parsing failed.
func ConfigFromBlock(block *common.Block) (*common.ConfigEnvelope, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) == 0 {
		return nil, errors.New("empty block")
	}
	txn := block.Data.Data[0]
	env, err := utils.GetEnvelopeFromBlock(txn)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	payload, err := utils.GetPayload(env)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if payload.Header == nil {
		return nil, errors.New("nil header in payload")
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if common.HeaderType(chdr.Type) != common.HeaderType_CONFIG {
		return nil, errNotAConfig
	}
	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrap(err, "invalid config envelope")
	}
	return configEnvelope, nil
}

// VerifyBlockHash verifies the hash chain of the block with the given index
// among the blocks of the given block buffer.
func VerifyBlockHash(indexInBuffer int, blockBuff []*common.Block) error {
	if len(blockBuff) <= indexInBuffer {
		return errors.Errorf("index %d out of bounds (total %d blocks)", indexInBuffer, len(blockBuff))
	}
	block := blockBuff[indexInBuffer]
	if block.Header == nil {
		return errors.New("missing block header")
	}
	seq := block.Header.Number
	dataHash := block.Data.Hash()
	// Verify data hash matches the hash in the header
	if !bytes.Equal(dataHash, block.Header.DataHash) {
		computedHash := hex.EncodeToString(dataHash)
		claimedHash := hex.EncodeToString(block.Header.DataHash)
		return errors.Errorf("computed hash of block (%d) (%s) doesn't match claimed hash (%s)",
			seq, computedHash, claimedHash)
	}
	// We have a previous block in the buffer, ensure current block's previous hash matches the previous one.
	if indexInBuffer > 0 {
		prevBlock := blockBuff[indexInBuffer-1]
		currSeq := block.Header.Number
		if prevBlock.Header == nil {
			return errors.New("previous block header is nil")
		}
		prevSeq := prevBlock.Header.Number
		if prevSeq+1 != currSeq {
			return errors.Errorf("sequences %d and %d were received consecutively", prevSeq, currSeq)
		}
		if !bytes.Equal(block.Header.PreviousHash, prevBlock.Header.Hash()) {
			claimedPrevHash := hex.EncodeToString(block.Header.PreviousHash)
			actualPrevHash := hex.EncodeToString(prevBlock.Header.Hash())
			return errors.Errorf("block %d's hash (%s) mismatches %d's prev block hash (%s)",
				currSeq, actualPrevHash, prevSeq, claimedPrevHash)
		}
	}
	return nil
}

// SignatureSetFromBlock creates a signature set out of a block.
func SignatureSetFromBlock(block *common.Block) ([]*common.SignedData, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_SIGNATURES) {
		return nil, errors.New("no metadata in block")
	}
	metadata, err := utils.GetMetadataFromBlock(block, common.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return nil, errors.Errorf("failed unmarshaling medatata for signatures: %v", err)
	}

	var signatureSet []*common.SignedData
	for _, metadataSignature := range metadata.Signatures {
		sigHdr, err := utils.GetSignatureHeader(metadataSignature.SignatureHeader)
		if err != nil {
			return nil, errors.Errorf("failed unmarshaling signature header for block with id %d: %v",
				block.Header.Number, err)
		}
		signatureSet = append(signatureSet,
			&common.SignedData{
				Identity: sigHdr.Creator,
				Data: util.ConcatenateBytes(metadata.Value,
					metadataSignature.SignatureHeader, block.Header.Bytes()),
				Signature: metadataSignature.Signature,
			},
		)
	}
	return signatureSet, nil
}

// VerifyBlockSignature verifies the signature on the block with the given BlockVerifier and the given config.
func VerifyBlockSignature(block *common.Block, verifier BlockVerifier, config *common.ConfigEnvelope) error {
	signatureSet, err := SignatureSetFromBlock(block)
	if err != nil {
		return err
	}
	return verifier.VerifyBlockSignature(signatureSet, config)
}

// EndpointConfig defines a configuration
// of endpoints of ordering service nodes
type EndpointConfig struct {
	TLSRootCAs [][]byte
	Endpoints  []string
}

// EndpointconfigFromConfigBlock retrieves TLS CA certificates and endpoints
// from a config block.
func EndpointconfigFromConfigBlock(block *common.Block) (*EndpointConfig, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	envelopeConfig, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, err
	}
	var tlsCACerts [][]byte
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed extracting bundle from envelope")
	}
	msps, err := bundle.MSPManager().GetMSPs()
	if err != nil {
		return nil, errors.Wrap(err, "failed obtaining MSPs from MSPManager")
	}
	ordererConfig, ok := bundle.OrdererConfig()
	if !ok {
		return nil, errors.New("failed obtaining orderer config from bundle")
	}
	for _, org := range ordererConfig.Organizations() {
		msp := msps[org.MSPID()]
		if msp == nil {
			return nil, errors.Errorf("no MSP found for MSP with ID of %s", org.MSPID())
		}
		tlsCACerts = append(tlsCACerts, msp.GetTLSRootCerts()...)
	}
	return &EndpointConfig{
		Endpoints:  bundle.ChannelConfig().OrdererAddresses(),
		TLSRootCAs: tlsCACerts,
	}, nil
}
