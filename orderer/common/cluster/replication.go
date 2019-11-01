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
	// RetryTimeout is the time the block puller retries.
	RetryTimeout = time.Second * 10
)

// ChannelPredicate accepts channels according to their names.
type ChannelPredicate func(channelName string) bool

// AnyChannel accepts all channels.
func AnyChannel(_ string) bool {
	return true
}

// PullerConfigFromTopLevelConfig creates a PullerConfig from a TopLevel config,
// and from a signer and TLS key cert pair.
// The PullerConfig's channel is initialized to be the system channel.
func PullerConfigFromTopLevelConfig(systemChannel string, conf *localconfig.TopLevel, tlsKey, tlsCert []byte, signer crypto.LocalSigner) PullerConfig {
	return PullerConfig{
		Channel:             systemChannel,
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
}

//go:generate mockery -dir . -name ChannelLister -case underscore -output mocks/

// ChannelLister returns a list of channels
type ChannelLister interface {
	// Channels returns a list of channels
	Channels() []ChannelGenesisBlock
	// Close closes the ChannelLister
	Close()
}

// Replicator replicates chains
type Replicator struct {
	DoNotPanicIfClusterNotReachable bool
	Filter                          ChannelPredicate
	SystemChannel                   string
	ChannelLister                   ChannelLister
	Logger                          *flogging.FabricLogger
	Puller                          *BlockPuller
	BootBlock                       *common.Block
	AmIPartOfChannel                SelfMembershipPredicate
	LedgerFactory                   LedgerFactory
}

// IsReplicationNeeded returns whether replication is needed,
// or the cluster node can resume standard boot flow.
func (r *Replicator) IsReplicationNeeded() (bool, error) {
	systemChannelLedger, err := r.LedgerFactory.GetOrCreate(r.SystemChannel)
	if err != nil {
		return false, err
	}

	height := systemChannelLedger.Height()
	var lastBlockSeq uint64
	// If Height is 0 then lastBlockSeq would be 2^64 - 1,
	// so make it 0 to take care of the overflow.
	if height == 0 {
		lastBlockSeq = 0
	} else {
		lastBlockSeq = height - 1
	}

	if r.BootBlock.Header.Number > lastBlockSeq {
		return true, nil
	}
	return false, nil
}

// ReplicateChains pulls chains and commits them.
// Returns the names of the chains replicated successfully.
func (r *Replicator) ReplicateChains() []string {
	var replicatedChains []string
	channels := r.discoverChannels()
	pullHints := r.channelsToPull(channels)
	totalChannelCount := len(pullHints.channelsToPull) + len(pullHints.channelsNotToPull)
	r.Logger.Info("Found myself in", len(pullHints.channelsToPull), "channels out of", totalChannelCount, ":", pullHints)

	// Append the genesis blocks of the application channels we have into the ledger
	for _, channels := range [][]ChannelGenesisBlock{pullHints.channelsToPull, pullHints.channelsNotToPull} {
		for _, channel := range channels {
			ledger, err := r.LedgerFactory.GetOrCreate(channel.ChannelName)
			if err != nil {
				r.Logger.Panicf("Failed to create a ledger for channel %s: %v", channel.ChannelName, err)
			}

			if channel.GenesisBlock == nil {
				if ledger.Height() == 0 {
					r.Logger.Panicf("Expecting channel %s to at least contain genesis block, but it doesn't", channel.ChannelName)
				}

				continue
			}

			gb, err := ChannelCreationBlockToGenesisBlock(channel.GenesisBlock)
			if err != nil {
				r.Logger.Panicf("Failed converting channel creation block for channel %s to genesis block: %v",
					channel.ChannelName, err)
			}
			r.appendBlock(gb, ledger, channel.ChannelName)
		}
	}

	for _, channel := range pullHints.channelsToPull {
		err := r.PullChannel(channel.ChannelName)
		if err == nil {
			replicatedChains = append(replicatedChains, channel.ChannelName)
		} else {
			r.Logger.Warningf("Failed pulling channel %s: %v", channel.ChannelName, err)
		}
	}

	// Last, pull the system chain.
	if err := r.PullChannel(r.SystemChannel); err != nil && err != ErrSkipped {
		r.Logger.Panicf("Failed pulling system channel: %v", err)
	}
	return replicatedChains
}

func (r *Replicator) discoverChannels() []ChannelGenesisBlock {
	r.Logger.Debug("Entering")
	defer r.Logger.Debug("Exiting")
	channels := GenesisBlocks(r.ChannelLister.Channels())
	r.Logger.Info("Discovered", len(channels), "channels:", channels.Names())
	r.ChannelLister.Close()
	return channels
}

// PullChannel pulls the given channel from some orderer,
// and commits it to the ledger.
func (r *Replicator) PullChannel(channel string) error {
	if !r.Filter(channel) {
		r.Logger.Infof("Channel %s shouldn't be pulled. Skipping it", channel)
		return ErrSkipped
	}
	r.Logger.Info("Pulling channel", channel)
	puller := r.Puller.Clone()
	defer puller.Close()
	puller.Channel = channel

	ledger, err := r.LedgerFactory.GetOrCreate(channel)
	if err != nil {
		r.Logger.Panicf("Failed to create a ledger for channel %s: %v", channel, err)
	}

	endpoint, latestHeight, _ := latestHeightAndEndpoint(puller)
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
	return r.pullChannelBlocks(channel, puller, latestHeight, ledger)
}

func (r *Replicator) pullChannelBlocks(channel string, puller *BlockPuller, latestHeight uint64, ledger LedgerWriter) error {
	nextBlockToPull := ledger.Height()
	if nextBlockToPull == latestHeight {
		r.Logger.Infof("Latest height found (%d) is equal to our height, skipping pulling channel %s", latestHeight, channel)
		return nil
	}
	// Pull the next block and remember its hash.
	nextBlock := puller.PullBlock(nextBlockToPull)
	if nextBlock == nil {
		return ErrRetryCountExhausted
	}
	r.appendBlock(nextBlock, ledger, channel)
	actualPrevHash := nextBlock.Header.Hash()

	for seq := uint64(nextBlockToPull + 1); seq < latestHeight; seq++ {
		block := puller.PullBlock(seq)
		if block == nil {
			return ErrRetryCountExhausted
		}
		reportedPrevHash := block.Header.PreviousHash
		if !bytes.Equal(reportedPrevHash, actualPrevHash) {
			return errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
				block.Header.Number, actualPrevHash, reportedPrevHash)
		}
		actualPrevHash = block.Header.Hash()
		if channel == r.SystemChannel && block.Header.Number == r.BootBlock.Header.Number {
			r.compareBootBlockWithSystemChannelLastConfigBlock(block)
			r.appendBlock(block, ledger, channel)
			// No need to pull further blocks from the system channel
			return nil
		}
		r.appendBlock(block, ledger, channel)
	}
	return nil
}

func (r *Replicator) appendBlock(block *common.Block, ledger LedgerWriter, channel string) {
	height := ledger.Height()
	if height > block.Header.Number {
		r.Logger.Infof("Skipping commit of block [%d] for channel %s because height is at %d", block.Header.Number, channel, height)
		return
	}
	if err := ledger.Append(block); err != nil {
		r.Logger.Panicf("Failed to write block [%d]: %v", block.Header.Number, err)
	}
	r.Logger.Infof("Committed block [%d] for channel %s", block.Header.Number, channel)
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

type channelPullHints struct {
	channelsToPull    []ChannelGenesisBlock
	channelsNotToPull []ChannelGenesisBlock
}

func (r *Replicator) channelsToPull(channels GenesisBlocks) channelPullHints {
	r.Logger.Info("Evaluating channels to pull:", channels.Names())

	// Backup the verifier of the puller
	verifier := r.Puller.VerifyBlockSequence
	// Restore it at the end of the function
	defer func() {
		r.Puller.VerifyBlockSequence = verifier
	}()
	// Set it to be a no-op verifier, because we can't verify latest blocks of channels.
	r.Puller.VerifyBlockSequence = func(blocks []*common.Block, channel string) error {
		return nil
	}

	var channelsNotToPull []ChannelGenesisBlock
	var channelsToPull []ChannelGenesisBlock
	for _, channel := range channels {
		r.Logger.Info("Probing whether I should pull channel", channel.ChannelName)
		puller := r.Puller.Clone()
		puller.Channel = channel.ChannelName
		// Disable puller buffering when we check whether we are in the channel,
		// as we only need to know about a single block.
		bufferSize := puller.MaxTotalBufferBytes
		puller.MaxTotalBufferBytes = 1
		err := Participant(puller, r.AmIPartOfChannel)
		puller.Close()
		// Restore the previous buffer size
		puller.MaxTotalBufferBytes = bufferSize
		if err == ErrNotInChannel || err == ErrForbidden {
			r.Logger.Infof("I do not belong to channel %s or am forbidden pulling it (%v), skipping chain retrieval", channel.ChannelName, err)
			channelsNotToPull = append(channelsNotToPull, channel)
			continue
		}
		if err == ErrServiceUnavailable {
			r.Logger.Infof("All orderers in the system channel are either down,"+
				"or do not service channel %s (%v), skipping chain retrieval", channel.ChannelName, err)
			channelsNotToPull = append(channelsNotToPull, channel)
			continue
		}
		if err == ErrRetryCountExhausted {
			r.Logger.Warningf("Could not obtain blocks needed for classifying whether I am in the channel,"+
				"skipping the retrieval of the chan %s", channel.ChannelName)
			channelsNotToPull = append(channelsNotToPull, channel)
			continue
		}
		if err != nil {
			if !r.DoNotPanicIfClusterNotReachable {
				r.Logger.Panicf("Failed classifying whether I belong to channel %s: %v, skipping chain retrieval", channel.ChannelName, err)
			}
			continue
		}
		r.Logger.Infof("I need to pull channel %s", channel.ChannelName)
		channelsToPull = append(channelsToPull, channel)
	}
	return channelPullHints{
		channelsToPull:    channelsToPull,
		channelsNotToPull: channelsNotToPull,
	}
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

//go:generate mockery -dir . -name VerifierRetriever -case underscore -output mocks/

// VerifierRetriever retrieves BlockVerifiers for channels.
type VerifierRetriever interface {
	// RetrieveVerifier retrieves a BlockVerifier for the given channel.
	RetrieveVerifier(channel string) BlockVerifier
}

// BlockPullerFromConfigBlock returns a BlockPuller that doesn't verify signatures on blocks.
func BlockPullerFromConfigBlock(conf PullerConfig, block *common.Block, verifierRetriever VerifierRetriever) (*BlockPuller, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	endpoints, err := EndpointconfigFromConfigBlock(block)
	if err != nil {
		return nil, err
	}

	clientConf := comm.ClientConfig{
		Timeout: conf.Timeout,
		SecOpts: &comm.SecureOptions{
			Certificate:       conf.TLSCert,
			Key:               conf.TLSKey,
			RequireClientCert: true,
			UseTLS:            true,
		},
	}

	dialer := &StandardDialer{
		ClientConfig: clientConf.Clone(),
	}

	tlsCertAsDER, _ := pem.Decode(conf.TLSCert)
	if tlsCertAsDER == nil {
		return nil, errors.Errorf("unable to decode TLS certificate PEM: %s", base64.StdEncoding.EncodeToString(conf.TLSCert))
	}

	return &BlockPuller{
		Logger:  flogging.MustGetLogger("orderer.common.cluster.replication"),
		Dialer:  dialer,
		TLSCert: tlsCertAsDER.Bytes,
		VerifyBlockSequence: func(blocks []*common.Block, channel string) error {
			verifier := verifierRetriever.RetrieveVerifier(channel)
			if verifier == nil {
				return errors.Errorf("couldn't acquire verifier for channel %s", channel)
			}
			return VerifyBlocks(blocks, verifier)
		},
		MaxTotalBufferBytes: conf.MaxTotalBufferBytes,
		Endpoints:           endpoints,
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
	HeightsByEndpoints() (map[string]uint64, error)

	// Close closes the ChainPuller
	Close()
}

// ChainInspector walks over a chain
type ChainInspector struct {
	Logger          *flogging.FabricLogger
	Puller          ChainPuller
	LastConfigBlock *common.Block
}

// ErrSkipped denotes that replicating a chain was skipped
var ErrSkipped = errors.New("skipped")

// ErrForbidden denotes that an ordering node refuses sending blocks due to access control.
var ErrForbidden = errors.New("forbidden pulling the channel")

// ErrServiceUnavailable denotes that an ordering node is not servicing at the moment.
var ErrServiceUnavailable = errors.New("service unavailable")

// ErrNotInChannel denotes that an ordering node is not in the channel
var ErrNotInChannel = errors.New("not in the channel")

var ErrRetryCountExhausted = errors.New("retry attempts exhausted")

// SelfMembershipPredicate determines whether the caller is found in the given config block
type SelfMembershipPredicate func(configBlock *common.Block) error

// Participant returns whether the caller participates in the chain.
// It receives a ChainPuller that should already be calibrated for the chain,
// and a SelfMembershipPredicate that is used to detect whether the caller should service the chain.
// It returns nil if the caller participates in the chain.
// It may return:
// ErrNotInChannel in case the caller doesn't participate in the chain.
// ErrForbidden in case the caller is forbidden from pulling the block.
// ErrServiceUnavailable in case all orderers reachable cannot complete the request.
// ErrRetryCountExhausted in case no orderer is reachable.
func Participant(puller ChainPuller, analyzeLastConfBlock SelfMembershipPredicate) error {
	lastConfigBlock, err := PullLastConfigBlock(puller)
	if err != nil {
		return err
	}
	return analyzeLastConfBlock(lastConfigBlock)
}

// PullLastConfigBlock pulls the last configuration block, or returns an error on failure.
func PullLastConfigBlock(puller ChainPuller) (*common.Block, error) {
	endpoint, latestHeight, err := latestHeightAndEndpoint(puller)
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
	lastConfNumber, err := lastConfigFromBlock(lastBlock)
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

func latestHeightAndEndpoint(puller ChainPuller) (string, uint64, error) {
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

func lastConfigFromBlock(block *common.Block) (uint64, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return 0, errors.New("no metadata in block")
	}
	return utils.GetLastConfigIndexFromBlock(block)
}

// Close closes the ChainInspector
func (ci *ChainInspector) Close() {
	ci.Puller.Close()
}

// ChannelGenesisBlock wraps a Block and its channel name
type ChannelGenesisBlock struct {
	ChannelName  string
	GenesisBlock *common.Block
}

// GenesisBlocks aggregates several ChannelGenesisBlocks
type GenesisBlocks []ChannelGenesisBlock

// Names returns the channel names all ChannelGenesisBlocks
func (gbs GenesisBlocks) Names() []string {
	var res []string
	for _, gb := range gbs {
		res = append(res, gb.ChannelName)
	}
	return res
}

// Channels returns the list of ChannelGenesisBlocks
// for all channels. Each such ChannelGenesisBlock contains
// the genesis block of the channel.
func (ci *ChainInspector) Channels() []ChannelGenesisBlock {
	channels := make(map[string]ChannelGenesisBlock)
	lastConfigBlockNum := ci.LastConfigBlock.Header.Number
	var block *common.Block
	var prevHash []byte
	for seq := uint64(0); seq < lastConfigBlockNum; seq++ {
		block = ci.Puller.PullBlock(seq)
		if block == nil {
			ci.Logger.Panicf("Failed pulling block [%d] from the system channel", seq)
		}
		ci.validateHashPointer(block, prevHash)
		channel, err := IsNewChannelBlock(block)
		if err != nil {
			// If we failed to classify a block, something is wrong in the system chain
			// we're trying to pull, so abort.
			ci.Logger.Panicf("Failed classifying block [%d]: %s", seq, err)
			continue
		}
		// Set the previous hash for the next iteration
		prevHash = block.Header.Hash()
		if channel == "" {
			ci.Logger.Info("Block", seq, "doesn't contain a new channel")
			continue
		}
		ci.Logger.Info("Block", seq, "contains channel", channel)
		channels[channel] = ChannelGenesisBlock{
			ChannelName:  channel,
			GenesisBlock: block,
		}
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

func (ci *ChainInspector) validateHashPointer(block *common.Block, prevHash []byte) {
	if prevHash == nil {
		return
	}
	if bytes.Equal(block.Header.PreviousHash, prevHash) {
		return
	}
	ci.Logger.Panicf("Claimed previous hash of block [%d] is %x but actual previous hash is %x",
		block.Header.Number, block.Header.PreviousHash, prevHash)
}

func flattenChannelMap(m map[string]ChannelGenesisBlock) []ChannelGenesisBlock {
	var res []ChannelGenesisBlock
	for _, csb := range m {
		res = append(res, csb)
	}
	return res
}

// ChannelCreationBlockToGenesisBlock converts a channel creation block to a genesis block
func ChannelCreationBlockToGenesisBlock(block *common.Block) (*common.Block, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	env, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, err
	}
	payload, err := utils.ExtractPayload(env)
	if err != nil {
		return nil, err
	}
	block.Data.Data = [][]byte{payload.Data}
	block.Header.DataHash = block.Data.Hash()
	block.Header.Number = 0
	block.Header.PreviousHash = nil
	metadata := &common.BlockMetadata{
		Metadata: make([][]byte, 4),
	}
	block.Metadata = metadata
	metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: 0}),
		// This is a genesis block, peer never verify this signature because we can't bootstrap
		// trust from an earlier block, hence there are no signatures here.
	})
	return block, nil
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
