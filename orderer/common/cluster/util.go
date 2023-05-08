/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// ConnByCertMap maps certificates represented as strings
// to gRPC connections
type ConnByCertMap map[string]*grpc.ClientConn

// Lable used for TLS Export Keying Material call
const KeyingMaterialLabel = "orderer v3 authentication label"

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

// Size returns the size of the connections by certificate mapping
func (cbc ConnByCertMap) Size() int {
	return len(cbc)
}

// CertificateComparator returns whether some relation holds for two given certificates
type CertificateComparator func([]byte, []byte) bool

// MemberMapping defines NetworkMembers by their ID
// and enables to lookup stubs by a certificate
type MemberMapping struct {
	id2stub       map[uint64]*Stub
	SamePublicKey CertificateComparator
}

// Foreach applies the given function on all stubs in the mapping
func (mp *MemberMapping) Foreach(f func(id uint64, stub *Stub)) {
	for id, stub := range mp.id2stub {
		f(id, stub)
	}
}

// Put inserts the given stub to the MemberMapping
func (mp *MemberMapping) Put(stub *Stub) {
	mp.id2stub[stub.ID] = stub
}

// Remove removes the stub with the given ID from the MemberMapping
func (mp *MemberMapping) Remove(ID uint64) {
	delete(mp.id2stub, ID)
}

// ByID retrieves the Stub with the given ID from the MemberMapping
func (mp MemberMapping) ByID(ID uint64) *Stub {
	return mp.id2stub[ID]
}

// LookupByClientCert retrieves a Stub with the given client certificate
func (mp MemberMapping) LookupByClientCert(cert []byte) *Stub {
	for _, stub := range mp.id2stub {
		if mp.SamePublicKey(stub.ClientTLSCert, cert) {
			return stub
		}
	}
	return nil
}

// LookupByIdentity retrieves a Stub by Identity
func (mp MemberMapping) LookupByIdentity(identity []byte) *Stub {
	for _, stub := range mp.id2stub {
		if bytes.Equal(identity, stub.Identity) {
			return stub
		}
	}
	return nil
}

// ServerCertificates returns a set of the server certificates
// represented as strings
func (mp MemberMapping) ServerCertificates() StringSet {
	res := make(StringSet)
	for _, member := range mp.id2stub {
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
	lock   sync.RWMutex
	Config comm.ClientConfig
}

func (dialer *PredicateDialer) UpdateRootCAs(serverRootCAs [][]byte) {
	dialer.lock.Lock()
	defer dialer.lock.Unlock()
	dialer.Config.SecOpts.ServerRootCAs = serverRootCAs
}

// Dial creates a new gRPC connection that can only be established, if the remote node's
// certificate chain satisfy verifyFunc
func (dialer *PredicateDialer) Dial(address string, verifyFunc RemoteVerifier) (*grpc.ClientConn, error) {
	dialer.lock.RLock()
	clientConfigCopy := dialer.Config
	dialer.lock.RUnlock()

	clientConfigCopy.SecOpts.VerifyCertificate = verifyFunc
	return clientConfigCopy.Dial(address)
}

// DERtoPEM returns a PEM representation of the DER
// encoded certificate
func DERtoPEM(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
}

// StandardDialer wraps an ClientConfig, and provides
// a means to connect according to given EndpointCriteria.
type StandardDialer struct {
	Config comm.ClientConfig
}

// Dial dials an address according to the given EndpointCriteria
func (dialer *StandardDialer) Dial(endpointCriteria EndpointCriteria) (*grpc.ClientConn, error) {
	clientConfigCopy := dialer.Config
	clientConfigCopy.SecOpts.ServerRootCAs = endpointCriteria.TLSRootCAs

	return clientConfigCopy.Dial(endpointCriteria.Endpoint)
}

// BlockSequenceVerifier verifies that the given consecutive sequence
// of blocks is valid.
type BlockSequenceVerifier func(blocks []*common.Block, channel string) error

// Dialer creates a gRPC connection to a remote address
type Dialer interface {
	Dial(endpointCriteria EndpointCriteria) (*grpc.ClientConn, error)
}

var errNotAConfig = errors.New("not a config block")

// ConfigFromBlock returns a ConfigEnvelope if exists, or a *NotAConfigBlock error.
// It may also return some other error in case parsing failed.
func ConfigFromBlock(block *common.Block) (*common.ConfigEnvelope, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) == 0 {
		return nil, errors.New("empty block")
	}
	txn := block.Data.Data[0]
	env, err := protoutil.GetEnvelopeFromBlock(txn)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	payload, err := protoutil.UnmarshalPayload(env.Payload)
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
	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
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
	dataHash := protoutil.BlockDataHash(block.Data)
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
		if !bytes.Equal(block.Header.PreviousHash, protoutil.BlockHeaderHash(prevBlock.Header)) {
			claimedPrevHash := hex.EncodeToString(block.Header.PreviousHash)
			actualPrevHash := hex.EncodeToString(protoutil.BlockHeaderHash(prevBlock.Header))
			return errors.Errorf("block [%d]'s hash (%s) mismatches block [%d]'s prev block hash (%s)",
				prevSeq, actualPrevHash, currSeq, claimedPrevHash)
		}
	}
	return nil
}

// VerifyBlockSignature verifies the signature on the block with the given BlockVerifier and the given config.
func VerifyBlockSignature(block *common.Block, verifier protoutil.BlockVerifierFunc) error {
	return verifier(block.Header, block.Metadata)
}

// EndpointCriteria defines criteria of how to connect to a remote orderer node.
type EndpointCriteria struct {
	Endpoint   string   // Endpoint of the form host:port
	TLSRootCAs [][]byte // PEM encoded TLS root CA certificates
}

// String returns a string representation of this EndpointCriteria
func (ep EndpointCriteria) String() string {
	var formattedCAs []interface{}
	for _, rawCAFile := range ep.TLSRootCAs {
		var bl *pem.Block
		pemContent := rawCAFile
		for {
			bl, pemContent = pem.Decode(pemContent)
			if bl == nil {
				break
			}
			cert, err := x509.ParseCertificate(bl.Bytes)
			if err != nil {
				break
			}

			issuedBy := cert.Issuer.String()
			if cert.Issuer.String() == cert.Subject.String() {
				issuedBy = "self"
			}

			info := make(map[string]interface{})
			info["Expired"] = time.Now().After(cert.NotAfter)
			info["Subject"] = cert.Subject.String()
			info["Issuer"] = issuedBy
			formattedCAs = append(formattedCAs, info)
		}
	}

	formattedEndpointCriteria := make(map[string]interface{})
	formattedEndpointCriteria["Endpoint"] = ep.Endpoint
	formattedEndpointCriteria["CAs"] = formattedCAs

	rawJSON, err := json.Marshal(formattedEndpointCriteria)
	if err != nil {
		return fmt.Sprintf("{\"Endpoint\": \"%s\"}", ep.Endpoint)
	}

	return string(rawJSON)
}

// EndpointconfigFromConfigBlock retrieves TLS CA certificates and endpoints
// from a config block.
func EndpointconfigFromConfigBlock(block *common.Block, bccsp bccsp.BCCSP) ([]EndpointCriteria, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	envelopeConfig, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, err
	}

	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig, bccsp)
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

func BlockVerifierBuilder(bccsp bccsp.BCCSP) func(block *common.Block) protoutil.BlockVerifierFunc {
	return func(block *common.Block) protoutil.BlockVerifierFunc {
		bundle, failed := bundleFromConfigBlock(block, bccsp)
		if failed != nil {
			return failed
		}

		policy, exists := bundle.PolicyManager().GetPolicy(policies.BlockValidation)
		if !exists {
			return createErrorFunc(errors.New("no policies in config block"))
		}

		bftEnabled := bundle.ChannelConfig().Capabilities().ConsensusTypeBFT()

		var consenters []*common.Consenter
		if bftEnabled {
			cfg, ok := bundle.OrdererConfig()
			if !ok {
				return createErrorFunc(errors.New("no orderer section in config block"))
			}
			consenters = cfg.Consenters()
		}

		return protoutil.BlockSignatureVerifier(bftEnabled, consenters, policy)
	}
}

func bundleFromConfigBlock(block *common.Block, bccsp bccsp.BCCSP) (*channelconfig.Bundle, protoutil.BlockVerifierFunc) {
	if block.Data == nil || len(block.Data.Data) == 0 {
		return nil, createErrorFunc(errors.New("block contains no data"))
	}

	env := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], env); err != nil {
		return nil, createErrorFunc(err)
	}

	bundle, err := channelconfig.NewBundleFromEnvelope(env, bccsp)
	if err != nil {
		return nil, createErrorFunc(err)
	}

	return bundle, nil
}

func createErrorFunc(err error) protoutil.BlockVerifierFunc {
	return func(_ *common.BlockHeader, _ *common.BlockMetadata) error {
		return errors.Wrap(err, "initialized with an invalid config block")
	}
}

//go:generate mockery --dir . --name VerifierFactory --case underscore --output ./mocks/

// VerifierFactory creates BlockVerifiers.
type VerifierFactory interface {
	// VerifierFromConfig creates a BlockVerifier from the given configuration.
	VerifierFromConfig(configuration *common.ConfigEnvelope, channel string) (protoutil.BlockVerifierFunc, error)
}

// VerificationRegistry registers verifiers and retrieves them.
type VerificationRegistry struct {
	LoadVerifier       func(chain string) protoutil.BlockVerifierFunc
	Logger             *flogging.FabricLogger
	VerifierFactory    VerifierFactory
	VerifiersByChannel map[string]protoutil.BlockVerifierFunc
}

//go:generate mockery --dir . --name ChainPuller --case underscore --output mocks/

// ChainPuller pulls blocks from a chain
type ChainPuller interface {
	// PullBlock pulls the given block from some orderer node
	PullBlock(seq uint64) *common.Block

	// HeightsByEndpoints returns the block heights by endpoints of orderers
	HeightsByEndpoints() (map[string]uint64, error)

	// Close closes the ChainPuller
	Close()
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

// RetrieveVerifier returns a BlockVerifierFunc for the given channel, or nil if not found.
func (vr *VerificationRegistry) RetrieveVerifier(channel string) protoutil.BlockVerifierFunc {
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

// BlockVerifierAssembler creates a BlockVerifier out of a config envelope
type BlockVerifierAssembler struct {
	Logger *flogging.FabricLogger
	BCCSP  bccsp.BCCSP
}

// VerifierFromConfig creates a BlockVerifier from the given configuration.
func (bva *BlockVerifierAssembler) VerifierFromConfig(configuration *common.ConfigEnvelope, channel string) (protoutil.BlockVerifierFunc, error) {
	bundle, err := channelconfig.NewBundle(channel, configuration.Config, bva.BCCSP)
	if err != nil {
		return createErrorFunc(err), err
	}

	policy, exists := bundle.PolicyManager().GetPolicy(policies.BlockValidation)
	if !exists {
		err := errors.New("no policies in config block")
		return createErrorFunc(err), err
	}

	bftEnabled := bundle.ChannelConfig().Capabilities().ConsensusTypeBFT()

	var consenters []*common.Consenter
	if bftEnabled {
		cfg, ok := bundle.OrdererConfig()
		if !ok {
			err := errors.New("no orderer section in config block")
			return createErrorFunc(err), err
		}
		consenters = cfg.Consenters()
	}
	return protoutil.BlockSignatureVerifier(bftEnabled, consenters, policy), nil
}

// BlockValidationPolicyVerifier verifies signatures based on the block validation policy.
type BlockValidationPolicyVerifier struct {
	Logger    *flogging.FabricLogger
	Channel   string
	PolicyMgr policies.Manager
	BCCSP     bccsp.BCCSP
}

// VerifyBlockSignature verifies the signed data associated to a block, optionally with the given config envelope.
func (bv *BlockValidationPolicyVerifier) VerifyBlockSignature(sd []*protoutil.SignedData, envelope *common.ConfigEnvelope) error {
	policyMgr := bv.PolicyMgr
	// If the envelope passed isn't nil, we should use a different policy manager.
	if envelope != nil {
		bundle, err := channelconfig.NewBundle(bv.Channel, envelope.Config, bv.BCCSP)
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
	return policy.EvaluateSignedData(sd)
}

//go:generate mockery --dir . --name BlockRetriever --case underscore --output ./mocks/

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
	lastConfigBlockNum, err := protoutil.GetLastConfigIndexFromBlock(block)
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

// CachePublicKeyComparisons creates CertificateComparator that caches invocations based on input arguments.
// The given CertificateComparator must be a stateless function.
func CachePublicKeyComparisons(f CertificateComparator) CertificateComparator {
	m := &ComparisonMemoizer{
		MaxEntries: 4096,
		F:          f,
	}
	return m.Compare
}

// ComparisonMemoizer speeds up comparison computations by caching past invocations of a stateless function
type ComparisonMemoizer struct {
	// Configuration
	F          func(a, b []byte) bool
	MaxEntries uint16
	// Internal state
	cache map[arguments]bool
	lock  sync.RWMutex
	once  sync.Once
	rand  *rand.Rand
}

type arguments struct {
	a, b string
}

// Size returns the number of computations the ComparisonMemoizer currently caches.
func (cm *ComparisonMemoizer) Size() int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return len(cm.cache)
}

// Compare compares the given two byte slices.
// It may return previous computations for the given two arguments,
// otherwise it will compute the function F and cache the result.
func (cm *ComparisonMemoizer) Compare(a, b []byte) bool {
	cm.once.Do(cm.setup)
	key := arguments{
		a: string(a),
		b: string(b),
	}

	cm.lock.RLock()
	result, exists := cm.cache[key]
	cm.lock.RUnlock()

	if exists {
		return result
	}

	result = cm.F(a, b)

	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.shrinkIfNeeded()
	cm.cache[key] = result

	return result
}

func (cm *ComparisonMemoizer) shrinkIfNeeded() {
	for {
		currentSize := uint16(len(cm.cache))
		if currentSize < cm.MaxEntries {
			return
		}
		cm.shrink()
	}
}

func (cm *ComparisonMemoizer) shrink() {
	// Shrink the cache by 25% by removing every fourth element (on average)
	for key := range cm.cache {
		if cm.rand.Int()%4 != 0 {
			continue
		}
		delete(cm.cache, key)
	}
}

func (cm *ComparisonMemoizer) setup() {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	cm.cache = make(map[arguments]bool)
}

func requestAsString(request *orderer.StepRequest) string {
	switch t := request.GetPayload().(type) {
	case *orderer.StepRequest_SubmitRequest:
		if t.SubmitRequest == nil || t.SubmitRequest.Payload == nil {
			return fmt.Sprintf("Empty SubmitRequest: %v", t.SubmitRequest)
		}
		return fmt.Sprintf("SubmitRequest for channel %s with payload of size %d",
			t.SubmitRequest.Channel, len(t.SubmitRequest.Payload.Payload))
	case *orderer.StepRequest_ConsensusRequest:
		return fmt.Sprintf("ConsensusRequest for channel %s with payload of size %d",
			t.ConsensusRequest.Channel, len(t.ConsensusRequest.Payload))
	default:
		return fmt.Sprintf("unknown type: %v", request)
	}
}

func exportKM(cs tls.ConnectionState, label string, context []byte) ([]byte, error) {
	tlsBinding, err := cs.ExportKeyingMaterial(label, context, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed generating TLS Binding material")
	}
	return tlsBinding, nil
}

func GetSessionBindingHash(authReq *orderer.NodeAuthRequest) []byte {
	return util.ComputeSHA256(util.ConcatenateBytes(
		[]byte(strconv.FormatUint(uint64(authReq.Version), 10)),
		[]byte(authReq.Timestamp.String()),
		[]byte(strconv.FormatUint(authReq.FromId, 10)),
		[]byte(strconv.FormatUint(authReq.ToId, 10)),
		[]byte(authReq.Channel),
	))
}

func GetTLSSessionBinding(ctx context.Context, bindingPayload []byte) ([]byte, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("failed extracting stream context")
	}
	connState := peerInfo.AuthInfo.(credentials.TLSInfo).State

	tlsBinding, err := exportKM(connState, KeyingMaterialLabel, bindingPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed exporting keying material")
	}

	return tlsBinding, nil
}

func VerifySignature(identity, msgHash, signature []byte) error {
	block, _ := pem.Decode(identity)
	if block == nil {
		return errors.New("pem decoding failed")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "key extraction failed")
	}

	pubKey, isECDSA := cert.PublicKey.(*ecdsa.PublicKey)
	if !isECDSA {
		return errors.New("not valid public key")
	}

	validSignature := ecdsa.VerifyASN1(pubKey, msgHash, signature)

	if !validSignature {
		return errors.New("signature invalid")
	}
	return nil
}

func SHA256Digest(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// VerifyBlocksBFT verifies the given consecutive sequence of blocks is valid, always verifies signature,
// and returns nil if it's valid, else an error.
func VerifyBlocksBFT(blocks []*common.Block, signatureVerifier protoutil.BlockVerifierFunc, vb protoutil.VerifierBuilder) error {
	return verifyBlockSequence(blocks, signatureVerifier, vb)
}

func verifyBlockSequence(blockBuff []*common.Block, signatureVerifier protoutil.BlockVerifierFunc, vb protoutil.VerifierBuilder) error {
	if len(blockBuff) == 0 {
		return errors.New("buffer is empty")
	}

	// Verify all configuration blocks that are found inside the block batch,
	// with the configuration that was committed (nil) or with one that is picked up
	// during iteration over the block batch.
	for _, block := range blockBuff {
		configFromBlock, err := ConfigFromBlock(block)

		if err != nil && err != errNotAConfig {
			return err
		}

		if err := VerifyBlockSignature(block, signatureVerifier); err != nil {
			// Genesis blocks are not signed, so silently ignore the error
			if block.Header.Number > 0 {
				return err
			}
		}

		if configFromBlock != nil {
			signatureVerifier = vb(block)
		}
	}

	return nil
}

// PullLastConfigBlock pulls the last configuration block, or returns an error on failure.
func PullLastConfigBlock(puller ChainPuller) (*common.Block, error) {
	endpoint, latestHeight, err := LatestHeightAndEndpoint(puller)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, ErrRetryCountExhausted
	}
	lastBlock := puller.PullBlock(latestHeight - 1)
	if lastBlock == nil {
		return nil, ErrRetryCountExhausted
	}
	lastConfNumber, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return nil, err
	}
	// The last config block is smaller than the latest height,
	// and a block iterator on the server side is a sequenced one.
	// So we need to reset the puller if we wish to pull an earlier block.
	puller.Close()
	lastConfigBlock := puller.PullBlock(lastConfNumber)
	if lastConfigBlock == nil {
		return nil, ErrRetryCountExhausted
	}
	return lastConfigBlock, nil
}

func LatestHeightAndEndpoint(puller ChainPuller) (string, uint64, error) {
	var maxHeight uint64
	var mostUpToDateEndpoint string
	heightsByEndpoints, err := puller.HeightsByEndpoints()
	if err != nil {
		return "", 0, err
	}
	for endpoint, height := range heightsByEndpoints {
		if height >= maxHeight {
			maxHeight = height
			mostUpToDateEndpoint = endpoint
		}
	}
	return mostUpToDateEndpoint, maxHeight, nil
}
