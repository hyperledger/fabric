/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/tools/protolator"
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

func (cbc ConnByCertMap) Size() int {
	return len(cbc)
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
	lock sync.RWMutex
	comm.ClientConfig
}

func (dialer *PredicateDialer) UpdateRootCAs(serverRootCAs [][]byte) {
	dialer.lock.Lock()
	defer dialer.lock.Unlock()
	dialer.ClientConfig.SecOpts.ServerRootCAs = serverRootCAs
}

// Dial creates a new gRPC connection that can only be established, if the remote node's
// certificate chain satisfy verifyFunc
func (dialer *PredicateDialer) Dial(address string, verifyFunc RemoteVerifier) (*grpc.ClientConn, error) {
	dialer.lock.RLock()
	cfg := dialer.ClientConfig.Clone()
	dialer.lock.RUnlock()

	cfg.SecOpts.VerifyCertificate = verifyFunc
	client, err := comm.NewGRPCClient(cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return client.NewConnection(address, "", func(tlsConfig *tls.Config) {
		// We need to dynamically overwrite the TLS root CAs,
		// as they may be updated.
		dialer.lock.RLock()
		serverRootCAs := dialer.ClientConfig.Clone().SecOpts.ServerRootCAs
		dialer.lock.RUnlock()

		tlsConfig.RootCAs = x509.NewCertPool()
		for _, pem := range serverRootCAs {
			tlsConfig.RootCAs.AppendCertsFromPEM(pem)
		}
	})
}

// DERtoPEM returns a PEM representation of the DER
// encoded certificate
func DERtoPEM(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
}

// StandardDialer wraps an AtomicClientConfig,
// and provides a means to connect according to given EndpointCriteria.
type StandardDialer struct {
	comm.ClientConfig
}

// Dial dials an address according to the given EndpointCriteria
func (dialer *StandardDialer) Dial(endpointCriteria EndpointCriteria) (*grpc.ClientConn, error) {
	cfg := dialer.ClientConfig.Clone()
	cfg.SecOpts.ServerRootCAs = endpointCriteria.TLSRootCAs

	client, err := comm.NewGRPCClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating gRPC client")
	}

	return client.NewConnection(endpointCriteria.Endpoint, "")
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
type BlockSequenceVerifier func(blocks []*common.Block, channel string) error

// Dialer creates a gRPC connection to a remote address
type Dialer interface {
	Dial(endpointCriteria EndpointCriteria) (*grpc.ClientConn, error)
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
	var isLastBlockConfigBlock bool
	// Verify all configuration blocks that are found inside the block batch,
	// with the configuration that was committed (nil) or with one that is picked up
	// during iteration over the block batch.
	for _, block := range blockBuff {
		configFromBlock, err := ConfigFromBlock(block)
		if err == errNotAConfig {
			isLastBlockConfigBlock = false
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
		isLastBlockConfigBlock = true
	}

	// Verify the last block's signature
	lastBlock := blockBuff[len(blockBuff)-1]

	// If last block is a config block, we verified it using the policy of the previous block, so it's valid.
	if isLastBlockConfigBlock {
		return nil
	}

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
	if block.Header.Number == 0 {
		configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
		if err != nil {
			return nil, errors.Wrap(err, "invalid config envelope")
		}
		return configEnvelope, nil
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
			return errors.Errorf("block [%d]'s hash (%s) mismatches %d's prev block hash (%s)",
				prevSeq, actualPrevHash, currSeq, claimedPrevHash)
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

// EndpointCriteria defines criteria of how to connect to a remote orderer node.
type EndpointCriteria struct {
	Endpoint   string   // Endpoint of the form host:port
	TLSRootCAs [][]byte // PEM encoded TLS root CA certificates
}

// EndpointconfigFromConfigBlock retrieves TLS CA certificates and endpoints
// from a config block.
func EndpointconfigFromConfigBlock(block *common.Block) ([]EndpointCriteria, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	envelopeConfig, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, err
	}

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

	mspIDsToCACerts := make(map[string][][]byte)
	var aggregatedTLSCerts [][]byte
	for _, org := range ordererConfig.Organizations() {
		// Validate that every orderer org has a corresponding MSP instance in the MSP Manager.
		msp, exists := msps[org.MSPID()]
		if !exists {
			return nil, errors.Errorf("no MSP found for MSP with ID of %s", org.MSPID())
		}

		// Build a per org mapping of the TLS CA certs for this org,
		// and aggregate all TLS CA certs into aggregatedTLSCerts to be used later on.
		var caCerts [][]byte
		caCerts = append(caCerts, msp.GetTLSIntermediateCerts()...)
		caCerts = append(caCerts, msp.GetTLSRootCerts()...)
		mspIDsToCACerts[org.MSPID()] = caCerts
		aggregatedTLSCerts = append(aggregatedTLSCerts, caCerts...)
	}

	endpointsPerOrg := perOrgEndpoints(ordererConfig, mspIDsToCACerts)
	if len(endpointsPerOrg) > 0 {
		return endpointsPerOrg, nil
	}

	return globalEndpointsFromConfig(aggregatedTLSCerts, bundle), nil
}

func perOrgEndpoints(ordererConfig channelconfig.Orderer, mspIDsToCerts map[string][][]byte) []EndpointCriteria {
	var endpointsPerOrg []EndpointCriteria

	for _, org := range ordererConfig.Organizations() {
		for _, endpoint := range org.Endpoints() {
			endpointsPerOrg = append(endpointsPerOrg, EndpointCriteria{
				TLSRootCAs: mspIDsToCerts[org.MSPID()],
				Endpoint:   endpoint,
			})
		}
	}

	return endpointsPerOrg
}

func globalEndpointsFromConfig(aggregatedTLSCerts [][]byte, bundle *channelconfig.Bundle) []EndpointCriteria {
	var globalEndpoints []EndpointCriteria
	for _, endpoint := range bundle.ChannelConfig().OrdererAddresses() {
		globalEndpoints = append(globalEndpoints, EndpointCriteria{
			Endpoint:   endpoint,
			TLSRootCAs: aggregatedTLSCerts,
		})
	}
	return globalEndpoints
}

//go:generate mockery -dir . -name VerifierFactory -case underscore -output ./mocks/

// VerifierFactory creates BlockVerifiers.
type VerifierFactory interface {
	// VerifierFromConfig creates a BlockVerifier from the given configuration.
	VerifierFromConfig(configuration *common.ConfigEnvelope, channel string) (BlockVerifier, error)
}

// VerificationRegistry registers verifiers and retrieves them.
type VerificationRegistry struct {
	LoadVerifier       func(chain string) BlockVerifier
	Logger             *flogging.FabricLogger
	VerifierFactory    VerifierFactory
	VerifiersByChannel map[string]BlockVerifier
}

// RegisterVerifier adds a verifier into the registry if applicable.
func (vr *VerificationRegistry) RegisterVerifier(chain string) {
	if _, exists := vr.VerifiersByChannel[chain]; exists {
		vr.Logger.Debugf("No need to register verifier for chain %s", chain)
		return
	}

	v := vr.LoadVerifier(chain)
	if v == nil {
		vr.Logger.Errorf("Failed loading verifier for chain %s", chain)
		return
	}

	vr.VerifiersByChannel[chain] = v
	vr.Logger.Infof("Registered verifier for chain %s", chain)
}

// RetrieveVerifier returns a BlockVerifier for the given channel, or nil if not found.
func (vr *VerificationRegistry) RetrieveVerifier(channel string) BlockVerifier {
	verifier, exists := vr.VerifiersByChannel[channel]
	if exists {
		return verifier
	}
	vr.Logger.Errorf("No verifier for channel %s exists", channel)
	return nil
}

// BlockCommitted notifies the VerificationRegistry upon a block commit, which may
// trigger a registration of a verifier out of the block in case the block is a config block.
func (vr *VerificationRegistry) BlockCommitted(block *common.Block, channel string) {
	conf, err := ConfigFromBlock(block)
	// The block doesn't contain a config block, but is a valid block
	if err == errNotAConfig {
		vr.Logger.Debugf("Committed block [%d] for channel %s that is not a config block",
			block.Header.Number, channel)
		return
	}
	// The block isn't a valid block
	if err != nil {
		vr.Logger.Errorf("Failed parsing block of channel %s: %v, content: %s",
			channel, err, BlockToString(block))
		return
	}

	// The block contains a config block
	verifier, err := vr.VerifierFactory.VerifierFromConfig(conf, channel)
	if err != nil {
		vr.Logger.Errorf("Failed creating a verifier from a config block for channel %s: %v, content: %s",
			channel, err, BlockToString(block))
		return
	}

	vr.VerifiersByChannel[channel] = verifier

	vr.Logger.Debugf("Committed config block [%d] for channel %s", block.Header.Number, channel)
}

// BlockToString returns a string representation of this block.
func BlockToString(block *common.Block) string {
	buff := &bytes.Buffer{}
	protolator.DeepMarshalJSON(buff, block)
	return buff.String()
}

// BlockCommitFunc signals a block commit.
type BlockCommitFunc func(block *common.Block, channel string)

// LedgerInterceptor intercepts block commits.
type LedgerInterceptor struct {
	Channel              string
	InterceptBlockCommit BlockCommitFunc
	LedgerWriter
}

// Append commits a block into the ledger, and also fires the configured callback.
func (interceptor *LedgerInterceptor) Append(block *common.Block) error {
	defer interceptor.InterceptBlockCommit(block, interceptor.Channel)
	return interceptor.LedgerWriter.Append(block)
}

// BlockVerifierAssembler creates a BlockVerifier out of a config envelope
type BlockVerifierAssembler struct {
	Logger *flogging.FabricLogger
}

// VerifierFromConfig creates a BlockVerifier from the given configuration.
func (bva *BlockVerifierAssembler) VerifierFromConfig(configuration *common.ConfigEnvelope, channel string) (BlockVerifier, error) {
	bundle, err := channelconfig.NewBundle(channel, configuration.Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed extracting bundle from envelope")
	}
	policyMgr := bundle.PolicyManager()

	return &BlockValidationPolicyVerifier{
		Logger:    bva.Logger,
		PolicyMgr: policyMgr,
		Channel:   channel,
	}, nil
}

// BlockValidationPolicyVerifier verifies signatures based on the block validation policy.
type BlockValidationPolicyVerifier struct {
	Logger    *flogging.FabricLogger
	Channel   string
	PolicyMgr policies.Manager
}

// VerifyBlockSignature verifies the signed data associated to a block, optionally with the given config envelope.
func (bv *BlockValidationPolicyVerifier) VerifyBlockSignature(sd []*common.SignedData, envelope *common.ConfigEnvelope) error {
	policyMgr := bv.PolicyMgr
	// If the envelope passed isn't nil, we should use a different policy manager.
	if envelope != nil {
		bundle, err := channelconfig.NewBundle(bv.Channel, envelope.Config)
		if err != nil {
			buff := &bytes.Buffer{}
			protolator.DeepMarshalJSON(buff, envelope.Config)
			bv.Logger.Errorf("Failed creating a new bundle for channel %s, Config content is: %s", bv.Channel, buff.String())
			return err
		}
		bv.Logger.Infof("Initializing new PolicyManager for channel %s", bv.Channel)
		policyMgr = bundle.PolicyManager()
	}
	policy, exists := policyMgr.GetPolicy(policies.BlockValidation)
	if !exists {
		return errors.Errorf("policy %s wasn't found", policies.BlockValidation)
	}
	return policy.Evaluate(sd)
}

//go:generate mockery -dir . -name BlockRetriever -case underscore -output ./mocks/

// BlockRetriever retrieves blocks
type BlockRetriever interface {
	// Block returns a block with the given number,
	// or nil if such a block doesn't exist.
	Block(number uint64) *common.Block
}

// LastConfigBlock returns the last config block relative to the given block.
func LastConfigBlock(block *common.Block, blockRetriever BlockRetriever) (*common.Block, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	if blockRetriever == nil {
		return nil, errors.New("nil blockRetriever")
	}
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return nil, errors.New("no metadata in block")
	}
	lastConfigBlockNum, err := utils.GetLastConfigIndexFromBlock(block)
	if err != nil {
		return nil, err
	}
	lastConfigBlock := blockRetriever.Block(lastConfigBlockNum)
	if lastConfigBlock == nil {
		return nil, errors.Errorf("unable to retrieve last config block [%d]", lastConfigBlockNum)
	}
	return lastConfigBlock, nil
}

// StreamCountReporter reports the number of streams currently connected to this node
type StreamCountReporter struct {
	Metrics *Metrics
	count   uint32
}

func (scr *StreamCountReporter) Increment() {
	count := atomic.AddUint32(&scr.count, 1)
	scr.Metrics.reportStreamCount(count)
}

func (scr *StreamCountReporter) Decrement() {
	count := atomic.AddUint32(&scr.count, ^uint32(0))
	scr.Metrics.reportStreamCount(count)
}

type certificateExpirationCheck struct {
	minimumExpirationWarningInterval time.Duration
	expiresAt                        time.Time
	expirationWarningThreshold       time.Duration
	lastWarning                      time.Time
	nodeName                         string
	endpoint                         string
	alert                            func(string, ...interface{})
}

func (exp *certificateExpirationCheck) checkExpiration(currentTime time.Time, channel string) {
	timeLeft := exp.expiresAt.Sub(currentTime)
	if timeLeft > exp.expirationWarningThreshold {
		return
	}

	timeSinceLastWarning := currentTime.Sub(exp.lastWarning)
	if timeSinceLastWarning < exp.minimumExpirationWarningInterval {
		return
	}

	exp.alert("Certificate of %s from %s for channel %s expires in less than %v",
		exp.nodeName, exp.endpoint, channel, timeLeft)
	exp.lastWarning = currentTime
}
