/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// RuntimeConfig defines the configuration of the consensus
// that is related to runtime.
type RuntimeConfig struct {
	BFTConfig              types.Configuration
	isConfig               bool
	logger                 *flogging.FabricLogger
	id                     uint64
	LastCommittedBlockHash string
	RemoteNodes            []cluster.RemoteNode
	ID2Identities          NodeIdentitiesByID
	LastBlock              *cb.Block
	LastConfigBlock        *cb.Block
	Nodes                  []uint64
	consenters             []*cb.Consenter
}

// BlockCommitted updates the config from the block
func (rtc RuntimeConfig) BlockCommitted(block *cb.Block, bccsp bccsp.BCCSP) (RuntimeConfig, error) {
	if _, err := cluster.ConfigFromBlock(block); err == nil {
		return rtc.configBlockCommitted(block, bccsp)
	}
	return RuntimeConfig{
		consenters:             rtc.consenters,
		BFTConfig:              rtc.BFTConfig,
		id:                     rtc.id,
		logger:                 rtc.logger,
		LastCommittedBlockHash: hex.EncodeToString(protoutil.BlockHeaderHash(block.Header)),
		Nodes:                  rtc.Nodes,
		ID2Identities:          rtc.ID2Identities,
		RemoteNodes:            rtc.RemoteNodes,
		LastBlock:              block,
		LastConfigBlock:        rtc.LastConfigBlock,
	}, nil
}

func (rtc RuntimeConfig) configBlockCommitted(block *cb.Block, bccsp bccsp.BCCSP) (RuntimeConfig, error) {
	nodeConf, err := RemoteNodesFromConfigBlock(block, rtc.logger, bccsp)
	if err != nil {
		return rtc, errors.Wrap(err, "remote nodes cannot be computed, rejecting config block")
	}

	bftConfig, err := configBlockToBFTConfig(rtc.id, block, bccsp)
	if err != nil {
		return RuntimeConfig{}, err
	}

	return RuntimeConfig{
		consenters:             nodeConf.consenters,
		BFTConfig:              bftConfig,
		isConfig:               true,
		id:                     rtc.id,
		logger:                 rtc.logger,
		LastCommittedBlockHash: hex.EncodeToString(protoutil.BlockHeaderHash(block.Header)),
		Nodes:                  nodeConf.nodeIDs,
		ID2Identities:          nodeConf.id2Identities,
		RemoteNodes:            nodeConf.remoteNodes,
		LastBlock:              block,
		LastConfigBlock:        block,
	}, nil
}

func configBlockToBFTConfig(selfID uint64, block *cb.Block, bccsp bccsp.BCCSP) (types.Configuration, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) == 0 {
		return types.Configuration{}, errors.New("empty block")
	}

	env, err := protoutil.UnmarshalEnvelope(block.Data.Data[0])
	if err != nil {
		return types.Configuration{}, err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env, bccsp)
	if err != nil {
		return types.Configuration{}, err
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		return types.Configuration{}, errors.New("no orderer config")
	}

	consensusConfigOptions := &smartbft.Options{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), consensusConfigOptions); err != nil {
		return types.Configuration{}, err
	}

	return configFromMetadataOptions(selfID, consensusConfigOptions)
}

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// newBlockPuller creates a new block puller
func newBlockPuller(
	support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP) (BlockPuller, error) {
	verifyBlockSequence := func(blocks []*cb.Block, _ string) error {
		vb := cluster.BlockVerifierBuilder(bccsp)
		return cluster.VerifyBlocksBFT(blocks, support.SignatureVerifier(), vb)
	}

	stdDialer := &cluster.StandardDialer{
		Config: baseDialer.Config.Clone(),
	}
	stdDialer.Config.AsyncConnect = false
	stdDialer.Config.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the configuration,
	endpoints, err := etcdraft.EndpointconfigFromSupport(support, bccsp)
	if err != nil {
		return nil, err
	}

	der, _ := pem.Decode(stdDialer.Config.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.Config.SecOpts.Certificate))
	}

	bp := &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller"),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChannelID(),
		Dialer:              stdDialer,
	}

	return bp, nil
}

func getViewMetadataFromBlock(block *cb.Block) (*smartbftprotos.ViewMetadata, error) {
	if block.Header.Number == 0 {
		// Genesis block has no prior metadata so we just return an un-initialized metadata
		return new(smartbftprotos.ViewMetadata), nil
	}

	signatureMetadata := protoutil.GetMetadataFromBlockOrPanic(block, cb.BlockMetadataIndex_SIGNATURES)
	ordererMD := &cb.OrdererBlockMetadata{}
	if err := proto.Unmarshal(signatureMetadata.Value, ordererMD); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling OrdererBlockMetadata")
	}

	var viewMetadata smartbftprotos.ViewMetadata
	if err := proto.Unmarshal(ordererMD.ConsenterMetadata, &viewMetadata); err != nil {
		return nil, err
	}

	return &viewMetadata, nil
}

func configFromMetadataOptions(selfID uint64, options *smartbft.Options) (types.Configuration, error) {
	var err error

	config := types.DefaultConfig
	config.SelfID = (uint64)(selfID)

	if options == nil {
		return config, errors.New("config metadata options field is nil")
	}

	config.RequestBatchMaxCount = options.RequestBatchMaxCount
	config.RequestBatchMaxBytes = options.RequestBatchMaxBytes
	if config.RequestBatchMaxInterval, err = time.ParseDuration(options.RequestBatchMaxInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestBatchMaxInterval")
	}
	config.IncomingMessageBufferSize = options.IncomingMessageBufferSize
	config.RequestPoolSize = options.RequestPoolSize
	if config.RequestForwardTimeout, err = time.ParseDuration(options.RequestForwardTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestForwardTimeout")
	}
	if config.RequestComplainTimeout, err = time.ParseDuration(options.RequestComplainTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestComplainTimeout")
	}
	if config.RequestAutoRemoveTimeout, err = time.ParseDuration(options.RequestAutoRemoveTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestAutoRemoveTimeout")
	}
	if config.ViewChangeResendInterval, err = time.ParseDuration(options.ViewChangeResendInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeResendInterval")
	}
	if config.ViewChangeTimeout, err = time.ParseDuration(options.ViewChangeTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeTimeout")
	}
	if config.LeaderHeartbeatTimeout, err = time.ParseDuration(options.LeaderHeartbeatTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option LeaderHeartbeatTimeout")
	}
	config.LeaderHeartbeatCount = options.LeaderHeartbeatCount
	if config.CollectTimeout, err = time.ParseDuration(options.CollectTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option CollectTimeout")
	}
	config.SyncOnStart = options.SyncOnStart
	config.SpeedUpViewChange = options.SpeedUpViewChange

	config.LeaderRotation = false
	config.DecisionsPerLeader = 0

	if err = config.Validate(); err != nil {
		return config, errors.Wrap(err, "config validation failed")
	}

	config.RequestMaxBytes = 500 * 1024
	return config, nil
}

type request struct {
	sigHdr   *cb.SignatureHeader
	envelope *cb.Envelope
	chHdr    *cb.ChannelHeader
}

// RequestInspector inspects incomming requests and validates serialized identity
type RequestInspector struct {
	ValidateIdentityStructure func(identity *msp.SerializedIdentity) error
}

func (ri *RequestInspector) requestIDFromSigHeader(sigHdr *cb.SignatureHeader) (types.RequestInfo, error) {
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(sigHdr.Creator, sID); err != nil {
		return types.RequestInfo{}, errors.Wrap(err, "identity isn't an MSP Identity")
	}

	if err := ri.ValidateIdentityStructure(sID); err != nil {
		return types.RequestInfo{}, err
	}

	var preimage []byte
	preimage = append(preimage, sigHdr.Nonce...)
	preimage = append(preimage, sigHdr.Creator...)
	txID := sha256.Sum256(preimage)
	clientID := sha256.Sum256(sigHdr.Creator)
	return types.RequestInfo{
		ID:       hex.EncodeToString(txID[:]),
		ClientID: hex.EncodeToString(clientID[:]),
	}, nil
}

// RequestID unwraps the request info from the raw request
func (ri *RequestInspector) RequestID(rawReq []byte) types.RequestInfo {
	req, err := ri.unwrapReq(rawReq)
	if err != nil {
		return types.RequestInfo{}
	}
	reqInfo, err := ri.requestIDFromSigHeader(req.sigHdr)
	if err != nil {
		return types.RequestInfo{}
	}
	return reqInfo
}

func (ri *RequestInspector) unwrapReq(req []byte) (*request, error) {
	envelope, err := protoutil.UnmarshalEnvelope(req)
	if err != nil {
		return nil, err
	}
	payload := &cb.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling payload")
	}

	if payload.Header == nil {
		return nil, errors.Errorf("no header in payload")
	}

	sigHdr := &cb.SignatureHeader{}
	if err := proto.Unmarshal(payload.Header.SignatureHeader, sigHdr); err != nil {
		return nil, err
	}

	if len(payload.Header.ChannelHeader) == 0 {
		return nil, errors.New("no channel header in payload")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshaling channel header")
	}

	return &request{
		chHdr:    chdr,
		sigHdr:   sigHdr,
		envelope: envelope,
	}, nil
}

// RemoteNodesFromConfigBlock unmarshals the node config from the block metadata
func RemoteNodesFromConfigBlock(block *cb.Block, logger *flogging.FabricLogger, bccsp bccsp.BCCSP) (*nodeConfig, error) {
	env := &cb.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], env); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling envelope of config block")
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env, bccsp)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting a new bundle from envelope of config block")
	}

	channelMSPs, err := bundle.MSPManager().GetMSPs()
	if err != nil {
		return nil, errors.Wrap(err, "failed obtaining MSPs from MSPManager")
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		return nil, errors.New("no orderer config in config block")
	}

	configOptions := &smartbft.Options{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), configOptions); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	var nodeIDs []uint64
	var remoteNodes []cluster.RemoteNode
	id2Identies := map[uint64][]byte{}
	for _, consenter := range oc.Consenters() {
		sanitizedID, err := crypto.SanitizeIdentity(protoutil.MarshalOrPanic(&msp.SerializedIdentity{
			IdBytes: consenter.Identity,
			Mspid:   consenter.MspId,
		}))
		if err != nil {
			logger.Panicf("Failed to sanitize identity: %v [%s]", err, string(consenter.Identity))
		}
		id2Identies[(uint64)(consenter.Id)] = sanitizedID
		logger.Infof("%s %d ---> %s", bundle.ConfigtxValidator().ChannelID(), consenter.Id, string(consenter.Identity))

		nodeIDs = append(nodeIDs, (uint64)(consenter.Id))

		serverCertAsDER, err := pemToDER(consenter.ServerTlsCert, (uint64)(consenter.Id), "server", logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := pemToDER(consenter.ClientTlsCert, (uint64)(consenter.Id), "client", logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Validate certificate structure
		for _, cert := range [][]byte{serverCertAsDER, clientCertAsDER} {
			if _, err := x509.ParseCertificate(cert); err != nil {
				pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
				logger.Errorf("Invalid certificate: %s", string(pemBytes))
				return nil, err
			}
		}

		nodeMSP, exists := channelMSPs[consenter.MspId]
		if !exists {
			return nil, errors.Errorf("no MSP found for MSP with ID of %s", consenter.MspId)
		}

		var rootCAs [][]byte
		rootCAs = append(rootCAs, nodeMSP.GetTLSRootCerts()...)
		rootCAs = append(rootCAs, nodeMSP.GetTLSIntermediateCerts()...)

		sanitizedCert, err := crypto.SanitizeX509Cert(consenter.Identity)
		if err != nil {
			return nil, err
		}

		remoteNodes = append(remoteNodes, cluster.RemoteNode{
			NodeAddress: cluster.NodeAddress{
				ID:       (uint64)(consenter.Id),
				Endpoint: fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
			},

			NodeCerts: cluster.NodeCerts{
				ClientTLSCert: clientCertAsDER,
				ServerTLSCert: serverCertAsDER,
				ServerRootCA:  rootCAs,
				Identity:      sanitizedCert,
			},
		})
	}

	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})

	return &nodeConfig{
		consenters:    oc.Consenters(),
		remoteNodes:   remoteNodes,
		id2Identities: id2Identies,
		nodeIDs:       nodeIDs,
	}, nil
}

type nodeConfig struct {
	id2Identities NodeIdentitiesByID
	remoteNodes   []cluster.RemoteNode
	nodeIDs       []uint64
	consenters    []*cb.Consenter
}

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate struct {
	ConsenterCertificate []byte
	CryptoProvider       bccsp.BCCSP
}

// IsConsenterOfChannel returns whether the caller is a consenter of a channel
// by inspecting the given configuration block.
// It returns nil if true, else returns an error.
func (conCert ConsenterCertificate) IsConsenterOfChannel(configBlock *cb.Block) error {
	if configBlock == nil {
		return errors.New("nil block")
	}
	envelopeConfig, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig, conCert.CryptoProvider)
	if err != nil {
		return err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return errors.New("no orderer config in bundle")
	}
	if oc.ConsensusType() != "BFT" {
		return errors.New("not a SmartBFT config block")
	}

	for _, consenter := range oc.Consenters() {
		if bytes.Equal(conCert.ConsenterCertificate, consenter.ServerTlsCert) || bytes.Equal(conCert.ConsenterCertificate, consenter.ClientTlsCert) {
			return nil
		}
	}
	return cluster.ErrNotInChannel
}

type worker struct {
	work      [][]byte
	f         func([]byte)
	workerNum int
	id        int
}

func (w *worker) doWork() {
	// sanity check
	if w.workerNum == 0 {
		panic("worker number is not defined")
	}

	if w.f == nil {
		panic("worker function is not defined")
	}

	if len(w.work) == 0 {
		panic("work is not defined")
	}

	for i, datum := range w.work {
		if i%w.workerNum != w.id {
			continue
		}

		w.f(datum)
	}
}
