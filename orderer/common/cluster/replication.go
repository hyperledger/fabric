/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"time"

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const (
	// RetryTimeout is the time the block puller retries
	RetryTimeout = time.Second * 10
)

// PullerConfigFromTopLevelConfig creates a PullerConfig from a TopLevel config,
// and from a signer and TLS key cert pair.
// The PullerConfig's channel is initialized to be the system channel.
func PullerConfigFromTopLevelConfig(conf *localconfig.TopLevel, tlsKey, tlsCert []byte, signer crypto.LocalSigner) PullerConfig {
	return PullerConfig{
		Channel:             conf.General.SystemChannel,
		MaxTotalBufferBytes: conf.General.Cluster.ReplicationBufferSize,
		Timeout:             conf.General.Cluster.RPCTimeout,
		TLSKey:              tlsKey,
		TLSCert:             tlsCert,
		Signer:              signer,
	}
}

//go:generate mockery -dir . -name LedgerWriter -case underscore -output mocks/

// LedgerWriter allows the caller to write blocks and inspect the height
type LedgerWriter interface {
	// Append a new block to the ledger
	Append(block *common.Block) error

	// Height returns the number of blocks on the ledger
	Height() uint64
}

//go:generate mockery -dir . -name LedgerFactory -case underscore -output mocks/

// LedgerFactory retrieves or creates new ledgers by chainID
type LedgerFactory interface {
	// GetOrCreate gets an existing ledger (if it exists)
	// or creates it if it does not
	GetOrCreate(chainID string) (LedgerWriter, error)

	// Close releases all resources acquired by the factory
	Close()
}

//go:generate mockery -dir . -name ChannelLister -case underscore -output mocks/

// ChannelLister returns a list of channels
type ChannelLister interface {
	// Channels returns a list of channels
	Channels() []string
	// Close closes the ChannelLister
	Close()
}

// Replicator replicates chains
type Replicator struct {
	SystemChannel    string
	ChannelLister    ChannelLister
	Logger           *flogging.FabricLogger
	Puller           *BlockPuller
	BootBlock        *common.Block
	AmIPartOfChannel selfMembershipPredicate
	LedgerFactory    LedgerFactory
}

// IsReplicationNeeded returns whether replication is needed,
// or the cluster node can resume standard boot flow.
func (r *Replicator) IsReplicationNeeded() (bool, error) {
	defer r.LedgerFactory.Close()
	systemChannelLedger, err := r.LedgerFactory.GetOrCreate(r.SystemChannel)
	if err != nil {
		return false, err
	}

	lastBlockSeq := systemChannelLedger.Height() - 1
	if r.BootBlock.Header.Number > lastBlockSeq {
		return true, nil
	}
	return false, nil
}

// ReplicateChains pulls chains and commits them.
func (r *Replicator) ReplicateChains() {
	channels := r.discoverChannels()
	channels2Pull := r.channelsToPull(channels)
	r.Logger.Info("Found myself in", len(channels2Pull), "channels:", channels2Pull)
	for _, channel := range channels2Pull {
		r.PullChannel(channel)
	}
	// Last, pull the system chain
	r.PullChannel(r.SystemChannel)
	r.LedgerFactory.Close()
}

func (r *Replicator) discoverChannels() []string {
	r.Logger.Debug("Entering")
	defer r.Logger.Debug("Exiting")
	channels := r.ChannelLister.Channels()
	r.Logger.Info("Discovered", len(channels), "channels:", channels)
	r.ChannelLister.Close()
	return channels
}

// PullChannel pulls the given channel from some orderer,
// and commits it to the ledger.
func (r *Replicator) PullChannel(channel string) error {
	r.Logger.Info("Pulling channel", channel)
	puller := r.Puller.Clone()
	defer puller.Close()
	puller.Channel = channel

	endpoint, latestHeight := latestHeightAndEndpoint(puller)
	if endpoint == "" {
		return errors.Errorf("failed obtaining the latest block for channel %s", channel)
	}
	r.Logger.Info("Latest block height for channel", channel, "is", latestHeight)
	// Ensure that if we pull the system channel, the latestHeight is bigger or equal to the
	// bootstrap block of the system channel.
	// Otherwise, we'd be left with a block gap.
	if channel == r.SystemChannel && latestHeight-1 < r.BootBlock.Header.Number {
		return errors.Errorf("latest height found among system channel(%s) orderers is %d, but the boot block's "+
			"sequence is %d", r.SystemChannel, latestHeight, r.BootBlock.Header.Number)
	}
	return r.pullChannelBlocks(channel, puller, latestHeight)
}

func (r *Replicator) pullChannelBlocks(channel string, puller ChainPuller, latestHeight uint64) error {
	ledger, err := r.LedgerFactory.GetOrCreate(channel)
	if err != nil {
		r.Logger.Panicf("Failed to create a ledger for channel %s: %v", channel, err)
	}
	// Pull the genesis block and remember its hash.
	genesisBlock := puller.PullBlock(0)
	r.appendBlock(genesisBlock, ledger)
	actualPrevHash := genesisBlock.Header.Hash()

	for seq := uint64(1); seq < latestHeight; seq++ {
		block := puller.PullBlock(seq)
		reportedPrevHash := block.Header.PreviousHash
		if !bytes.Equal(reportedPrevHash, actualPrevHash) {
			return errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
				block.Header.Number, actualPrevHash, reportedPrevHash)
		}
		actualPrevHash = block.Header.Hash()
		if channel == r.SystemChannel && block.Header.Number == r.BootBlock.Header.Number {
			r.compareBootBlockWithSystemChannelLastConfigBlock(block)
			r.appendBlock(block, ledger)
			// No need to pull further blocks from the system channel
			return nil
		}
		r.appendBlock(block, ledger)
	}
	return nil
}

func (r *Replicator) appendBlock(block *common.Block, ledger LedgerWriter) {
	if err := ledger.Append(block); err != nil {
		r.Logger.Panicf("Failed to write block %d: %v", block.Header.Number, err)
	}
}

func (r *Replicator) compareBootBlockWithSystemChannelLastConfigBlock(block *common.Block) {
	// Overwrite the received block's data hash
	block.Header.DataHash = block.Data.Hash()

	bootBlockHash := r.BootBlock.Header.Hash()
	retrievedBlockHash := block.Header.Hash()
	if bytes.Equal(bootBlockHash, retrievedBlockHash) {
		return
	}
	r.Logger.Panicf("Block header mismatch on last system channel block, expected %s, got %s",
		hex.EncodeToString(bootBlockHash), hex.EncodeToString(retrievedBlockHash))
}

func (r *Replicator) channelsToPull(channels []string) []string {
	r.Logger.Info("Will now pull channels:", channels)
	var channelsToPull []string
	for _, channel := range channels {
		r.Logger.Info("Pulling chain for", channel)
		puller := r.Puller.Clone()
		puller.Channel = channel
		// Disable puller buffering when we check whether we are in the channel,
		// as we only need to know about a single block.
		bufferSize := puller.MaxTotalBufferBytes
		puller.MaxTotalBufferBytes = 1
		err := Participant(puller, r.AmIPartOfChannel)
		puller.Close()
		// Restore the previous buffer size
		puller.MaxTotalBufferBytes = bufferSize
		if err == NotInChannelError {
			r.Logger.Info("I do not belong to channel", channel, ", skipping chain retrieval")
			continue
		}
		if err != nil {
			r.Logger.Panicf("Failed classifying whether I belong to channel %s: %v, skipping chain retrieval", channel, err)
			continue
		}
		channelsToPull = append(channelsToPull, channel)
	}
	return channelsToPull
}

// PullerConfig configures a BlockPuller.
type PullerConfig struct {
	TLSKey              []byte
	TLSCert             []byte
	Timeout             time.Duration
	Signer              crypto.LocalSigner
	Channel             string
	MaxTotalBufferBytes int
}

// BlockPullerFromConfigBlock returns a BlockPuller that doesn't verify signatures on blocks.
func BlockPullerFromConfigBlock(conf PullerConfig, block *common.Block) (*BlockPuller, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	endpointconfig, err := EndpointconfigFromConfigBlock(block)
	if err != nil {
		return nil, err
	}

	dialer := &StandardDialer{
		Dialer: NewTLSPinningDialer(comm.ClientConfig{
			Timeout: conf.Timeout,
			SecOpts: &comm.SecureOptions{
				ServerRootCAs:     endpointconfig.TLSRootCAs,
				Certificate:       conf.TLSCert,
				Key:               conf.TLSKey,
				RequireClientCert: true,
				UseTLS:            true,
			},
		})}

	tlsCertAsDER, _ := pem.Decode(conf.TLSCert)
	if tlsCertAsDER == nil {
		return nil, errors.Errorf("unable to decode TLS certificate PEM: %s", base64.StdEncoding.EncodeToString(conf.TLSCert))
	}

	return &BlockPuller{
		Logger:  flogging.MustGetLogger("orderer.common.cluster.replication"),
		Dialer:  dialer,
		TLSCert: tlsCertAsDER.Bytes,
		VerifyBlockSequence: func(blocks []*common.Block) error {
			return VerifyBlocks(blocks, &NoopBlockVerifier{})
		},
		MaxTotalBufferBytes: conf.MaxTotalBufferBytes,
		Endpoints:           endpointconfig.Endpoints,
		RetryTimeout:        RetryTimeout,
		FetchTimeout:        conf.Timeout,
		Channel:             conf.Channel,
		Signer:              conf.Signer,
	}, nil
}

// NoopBlockVerifier doesn't verify block signatures
type NoopBlockVerifier struct{}

// VerifyBlockSignature accepts all signatures over blocks.
func (*NoopBlockVerifier) VerifyBlockSignature(sd []*common.SignedData, config *common.ConfigEnvelope) error {
	return nil
}

//go:generate mockery -dir . -name ChainPuller -case underscore -output mocks/

// ChainPuller pulls blocks from a chain
type ChainPuller interface {
	// PullBlock pulls the given block from some orderer node
	PullBlock(seq uint64) *common.Block

	// HeightsByEndpoints returns the block heights by endpoints of orderers
	HeightsByEndpoints() map[string]uint64

	// Close closes the ChainPuller
	Close()
}

// ChainInspector walks over a chain
type ChainInspector struct {
	Logger          *flogging.FabricLogger
	Puller          ChainPuller
	LastConfigBlock *common.Block
}

// NotInChannelError denotes that an ordering node is not in the channel
var NotInChannelError = errors.New("not in the channel")

// selfMembershipPredicate determines whether the caller is found in the given config block
type selfMembershipPredicate func(configBlock *common.Block) error

// Participant returns whether the caller participates in the chain.
// It receives a ChainPuller that should already be calibrated for the chain,
// and a selfMembershipPredicate that is used to detect whether the caller should service the chain.
// It returns nil if the caller participates in the chain.
// It may return notInChannelError error in case the caller doesn't participate in the chain.
func Participant(puller ChainPuller, analyzeLastConfBlock selfMembershipPredicate) error {
	endpoint, latestHeight := latestHeightAndEndpoint(puller)
	if endpoint == "" {
		return errors.New("no available orderer")
	}
	lastBlock := puller.PullBlock(latestHeight - 1)
	lastConfNumber, err := lastConfigFromBlock(lastBlock)
	if err != nil {
		return err
	}
	// The last config block is smaller than the latest height,
	// and a block iterator on the server side is a sequenced one.
	// So we need to reset the puller if we wish to pull an earlier block.
	puller.Close()
	lastConfigBlock := puller.PullBlock(lastConfNumber)
	return analyzeLastConfBlock(lastConfigBlock)
}

func latestHeightAndEndpoint(puller ChainPuller) (string, uint64) {
	var maxHeight uint64
	var mostUpToDateEndpoint string
	for endpoint, height := range puller.HeightsByEndpoints() {
		if height > maxHeight {
			maxHeight = height
			mostUpToDateEndpoint = endpoint
		}
	}
	return mostUpToDateEndpoint, maxHeight
}

func lastConfigFromBlock(block *common.Block) (uint64, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return 0, errors.New("no metadata in block")
	}
	return utils.GetLastConfigIndexFromBlock(block)
}

// Channels returns the list of channels
// that exist in the chain
func (ci *ChainInspector) Channels() []string {
	channels := make(map[string]struct{})
	lastConfigBlockNum := ci.LastConfigBlock.Header.Number
	var block *common.Block
	for seq := uint64(1); seq < lastConfigBlockNum; seq++ {
		block = ci.Puller.PullBlock(seq)
		channel, err := IsNewChannelBlock(block)
		if err != nil {
			// If we failed to classify a block, something is wrong in the system chain
			// we're trying to pull, so abort.
			ci.Logger.Panic("Failed classifying block", seq, ":", err)
			continue
		}
		if channel == "" {
			ci.Logger.Info("Block", seq, "doesn't contain a new channel")
			continue
		}
		ci.Logger.Info("Block", seq, "contains channel", channel)
		channels[channel] = struct{}{}
	}
	// At this point, block holds reference to the last block pulled.
	// We ensure that the hash of the last block pulled, is the previous hash
	// of the LastConfigBlock we were initialized with.
	// We don't need to verify the entire chain of all blocks we pulled,
	// because the block puller calls VerifyBlockHash on all blocks it pulls.
	last2Blocks := []*common.Block{block, ci.LastConfigBlock}
	if err := VerifyBlockHash(1, last2Blocks); err != nil {
		ci.Logger.Panic("System channel pulled doesn't match the boot last config block:", err)
	}

	return flattenChannelMap(channels)
}

func flattenChannelMap(m map[string]struct{}) []string {
	var res []string
	for channel := range m {
		res = append(res, channel)
	}
	return res
}

// IsNewChannelBlock returns a name of the channel in case
// it holds a channel create transaction, or empty string otherwise.
func IsNewChannelBlock(block *common.Block) (string, error) {
	if block == nil {
		return "", errors.New("nil block")
	}
	env, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return "", err
	}
	payload, err := utils.ExtractPayload(env)
	if err != nil {
		return "", err
	}
	if payload.Header == nil {
		return "", errors.New("nil header in payload")
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	// The transaction is an orderer transaction
	if common.HeaderType(chdr.Type) != common.HeaderType_ORDERER_TRANSACTION {
		return "", nil
	}
	systemChannelName := chdr.ChannelId
	innerEnvelope, err := utils.UnmarshalEnvelope(payload.Data)
	if err != nil {
		return "", err
	}
	innerPayload, err := utils.UnmarshalPayload(innerEnvelope.Payload)
	if err != nil {
		return "", err
	}
	if innerPayload.Header == nil {
		return "", errors.New("inner payload's header is nil")
	}
	chdr, err = utils.UnmarshalChannelHeader(innerPayload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	// The inner payload's header is a config transaction
	if common.HeaderType(chdr.Type) != common.HeaderType_CONFIG {
		return "", nil
	}
	// In any case, exclude all system channel transactions
	if chdr.ChannelId == systemChannelName {
		return "", nil
	}
	return chdr.ChannelId, nil
}
