/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// Semaphore provides to the validator means for synchronisation
type Semaphore interface {
	// Acquire implements semaphore-like acquire semantics
	Acquire(ctx context.Context) error

	// Release implements semaphore-like release semantics
	Release()
}

// ChannelResources provides access to channel artefacts or
// functions to interact with them
type ChannelResources interface {
	// MSPManager returns the MSP manager for this channel
	MSPManager() msp.MSPManager

	// Apply attempts to apply a configtx to become the new config
	Apply(configtx *common.ConfigEnvelope) error

	// GetMSPIDs returns the IDs for the application MSPs
	// that have been defined in the channel
	GetMSPIDs() []string

	// Capabilities defines the capabilities for the application portion of this channel
	Capabilities() channelconfig.ApplicationCapabilities
}

// LedgerResources provides access to ledger artefacts or
// functions to interact with them
type LedgerResources interface {
	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(txID string) (*peer.ProcessedTransaction, error)

	// NewQueryExecutor gives handle to a query executor.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// Dispatcher is an interface to decouple tx validator
// and plugin dispatcher
type Dispatcher interface {
	// Dispatch invokes the appropriate validation plugin for the supplied transaction in the block
	Dispatch(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (error, peer.TxValidationCode)
}

//go:generate mockery -dir . -name ChannelResources -case underscore -output mocks/
//go:generate mockery -dir . -name LedgerResources -case underscore -output mocks/
//go:generate mockery -dir . -name Dispatcher -case underscore -output mocks/

//go:generate mockery -dir . -name QueryExecutor -case underscore -output mocks/

// QueryExecutor is the local interface that used to generate mocks for foreign interface.
type QueryExecutor interface {
	ledger.QueryExecutor
}

//go:generate mockery -dir . -name ChannelPolicyManagerGetter -case underscore -output mocks/

// ChannelPolicyManagerGetter is the local interface that used to generate mocks for foreign interface.
type ChannelPolicyManagerGetter interface {
	policies.ChannelPolicyManagerGetter
}

//go:generate mockery -dir . -name PolicyManager -case underscore -output mocks/

type PolicyManager interface {
	policies.Manager
}

//go:generate mockery -dir plugindispatcher/ -name CollectionResources -case underscore -output mocks/

// TxValidator is the implementation of Validator interface, keeps
// reference to the ledger to enable tx simulation
// and execution of plugins
type TxValidator struct {
	ChainID          string
	Semaphore        Semaphore
	ChannelResources ChannelResources
	LedgerResources  LedgerResources
	Dispatcher       Dispatcher
}

var logger = flogging.MustGetLogger("committer.txvalidator")

type blockValidationRequest struct {
	block *common.Block
	d     []byte
	tIdx  int
}

type blockValidationResult struct {
	tIdx           int
	validationCode peer.TxValidationCode
	err            error
	txid           string
}

// NewTxValidator creates new transactions validator
func NewTxValidator(
	chainID string,
	sem Semaphore,
	cr ChannelResources,
	ler LedgerResources,
	lcr plugindispatcher.LifecycleResources,
	cor plugindispatcher.CollectionResources,
	pm plugin.Mapper,
	channelPolicyManagerGetter policies.ChannelPolicyManagerGetter,
) *TxValidator {
	// Encapsulates interface implementation
	pluginValidator := plugindispatcher.NewPluginValidator(pm, ler, &dynamicDeserializer{cr: cr}, &dynamicCapabilities{cr: cr}, channelPolicyManagerGetter, cor)
	return &TxValidator{
		ChainID:          chainID,
		Semaphore:        sem,
		ChannelResources: cr,
		LedgerResources:  ler,
		Dispatcher:       plugindispatcher.New(chainID, cr, ler, lcr, pluginValidator),
	}
}

func (v *TxValidator) chainExists(chain string) bool {
	// TODO: implement this function!
	return true
}

// Validate performs the validation of a block. The validation
// of each transaction in the block is performed in parallel.
// The approach is as follows: the committer thread starts the
// tx validation function in a goroutine (using a semaphore to cap
// the number of concurrent validating goroutines). The committer
// thread then reads results of validation (in orderer of completion
// of the goroutines) from the results channel. The goroutines
// perform the validation of the txs in the block and enqueue the
// validation result in the results channel. A few note-worthy facts:
// 1) to keep the approach simple, the committer thread enqueues
//    all transactions in the block and then moves on to reading the
//    results.
// 2) for parallel validation to work, it is important that the
//    validation function does not change the state of the system.
//    Otherwise the order in which validation is perform matters
//    and we have to resort to sequential validation (or some locking).
//    This is currently true, because the only function that affects
//    state is when a config transaction is received, but they are
//    guaranteed to be alone in the block. If/when this assumption
//    is violated, this code must be changed.
func (v *TxValidator) Validate(block *common.Block) error {
	var err error
	var errPos int

	startValidation := time.Now() // timer to log Validate block duration
	logger.Debugf("[%s] START Block Validation for block [%d]", v.ChainID, block.Header.Number)

	// Initialize trans as valid here, then set invalidation reason code upon invalidation below
	txsfltr := ledgerUtil.NewTxValidationFlags(len(block.Data.Data))
	// array of txids
	txidArray := make([]string, len(block.Data.Data))

	results := make(chan *blockValidationResult)
	go func() {
		for tIdx, d := range block.Data.Data {
			// ensure that we don't have too many concurrent validation workers
			v.Semaphore.Acquire(context.Background())

			go func(index int, data []byte) {
				defer v.Semaphore.Release()

				v.validateTx(&blockValidationRequest{
					d:     data,
					block: block,
					tIdx:  index,
				}, results)
			}(tIdx, d)
		}
	}()

	logger.Debugf("expecting %d block validation responses", len(block.Data.Data))

	// now we read responses in the order in which they come back
	for i := 0; i < len(block.Data.Data); i++ {
		res := <-results

		if res.err != nil {
			// if there is an error, we buffer its value, wait for
			// all workers to complete validation and then return
			// the error from the first tx in this block that returned an error
			logger.Debugf("got terminal error %s for idx %d", res.err, res.tIdx)

			if err == nil || res.tIdx < errPos {
				err = res.err
				errPos = res.tIdx
			}
		} else {
			// if there was no error, we set the txsfltr and we set the
			// txsChaincodeNames and txsUpgradedChaincodes maps
			logger.Debugf("got result for idx %d, code %d", res.tIdx, res.validationCode)

			txsfltr.SetFlag(res.tIdx, res.validationCode)

			if res.validationCode == peer.TxValidationCode_VALID {
				txidArray[res.tIdx] = res.txid
			}
		}
	}

	// if we're here, all workers have completed the validation.
	// If there was an error we return the error from the first
	// tx in this block that returned an error
	if err != nil {
		return err
	}

	// we mark invalid any transaction that has a txid
	// which is equal to that of a previous tx in this block
	markTXIdDuplicates(txidArray, txsfltr)

	// make sure no transaction has skipped validation
	err = v.allValidated(txsfltr, block)
	if err != nil {
		return err
	}

	// Initialize metadata structure
	protoutil.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsfltr

	elapsedValidation := time.Since(startValidation) / time.Millisecond // duration in ms
	logger.Infof("[%s] Validated block [%d] in %dms", v.ChainID, block.Header.Number, elapsedValidation)

	return nil
}

// allValidated returns error if some of the validation flags have not been set
// during validation
func (v *TxValidator) allValidated(txsfltr ledgerUtil.TxValidationFlags, block *common.Block) error {
	for id, f := range txsfltr {
		if peer.TxValidationCode(f) == peer.TxValidationCode_NOT_VALIDATED {
			return errors.Errorf("transaction %d in block %d has skipped validation", id, block.Header.Number)
		}
	}

	return nil
}

func markTXIdDuplicates(txids []string, txsfltr ledgerUtil.TxValidationFlags) {
	txidMap := make(map[string]struct{})

	for id, txid := range txids {
		if txid == "" {
			continue
		}

		_, in := txidMap[txid]
		if in {
			logger.Error("Duplicate txid", txid, "found, skipping")
			txsfltr.SetFlag(id, peer.TxValidationCode_DUPLICATE_TXID)
		} else {
			txidMap[txid] = struct{}{}
		}
	}
}

func (v *TxValidator) validateTx(req *blockValidationRequest, results chan<- *blockValidationResult) {
	block := req.block
	d := req.d
	tIdx := req.tIdx
	txID := ""

	if d == nil {
		results <- &blockValidationResult{
			tIdx: tIdx,
		}
		return
	}

	if env, err := protoutil.GetEnvelopeFromBlock(d); err != nil {
		logger.Warningf("Error getting tx from block: %+v", err)
		results <- &blockValidationResult{
			tIdx:           tIdx,
			validationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
		}
		return
	} else if env != nil {
		// validate the transaction: here we check that the transaction
		// is properly formed, properly signed and that the security
		// chain binding proposal to endorsements to tx holds. We do
		// NOT check the validity of endorsements, though. That's a
		// job for the validation plugins
		logger.Debugf("[%s] validateTx starts for block %p env %p txn %d", v.ChainID, block, env, tIdx)
		defer logger.Debugf("[%s] validateTx completes for block %p env %p txn %d", v.ChainID, block, env, tIdx)
		var payload *common.Payload
		var err error
		var txResult peer.TxValidationCode

		if payload, txResult = validation.ValidateTransaction(env, v.ChannelResources.Capabilities()); txResult != peer.TxValidationCode_VALID {
			logger.Errorf("Invalid transaction with index %d", tIdx)
			results <- &blockValidationResult{
				tIdx:           tIdx,
				validationCode: txResult,
			}
			return
		}

		chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			logger.Warningf("Could not unmarshal channel header, err %s, skipping", err)
			results <- &blockValidationResult{
				tIdx:           tIdx,
				validationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
			}
			return
		}

		channel := chdr.ChannelId
		logger.Debugf("Transaction is for channel %s", channel)

		if !v.chainExists(channel) {
			logger.Errorf("Dropping transaction for non-existent channel %s", channel)
			results <- &blockValidationResult{
				tIdx:           tIdx,
				validationCode: peer.TxValidationCode_TARGET_CHAIN_NOT_FOUND,
			}
			return
		}

		if common.HeaderType(chdr.Type) == common.HeaderType_ENDORSER_TRANSACTION {

			txID = chdr.TxId

			// Check duplicate transactions
			erroneousResultEntry := v.checkTxIdDupsLedger(tIdx, chdr, v.LedgerResources)
			if erroneousResultEntry != nil {
				results <- erroneousResultEntry
				return
			}

			// Validate tx with plugins
			logger.Debug("Validating transaction with plugins")
			err, cde := v.Dispatcher.Dispatch(tIdx, payload, d, block)
			if err != nil {
				logger.Errorf("Dispatch for transaction txId = %s returned error: %s", txID, err)
				switch err.(type) {
				case *commonerrors.VSCCExecutionFailureError:
					results <- &blockValidationResult{
						tIdx: tIdx,
						err:  err,
					}
					return
				case *commonerrors.VSCCInfoLookupFailureError:
					results <- &blockValidationResult{
						tIdx: tIdx,
						err:  err,
					}
					return
				default:
					results <- &blockValidationResult{
						tIdx:           tIdx,
						validationCode: cde,
					}
					return
				}
			}
		} else if common.HeaderType(chdr.Type) == common.HeaderType_TOKEN_TRANSACTION {

			txID = chdr.TxId
			if !v.ChannelResources.Capabilities().FabToken() {
				logger.Debugf("Unsupported transaction type [%s] in block number [%d] transaction index [%d]: FabToken capability is not enabled",
					common.HeaderType(chdr.Type), block.Header.Number, tIdx)
				results <- &blockValidationResult{
					tIdx:           tIdx,
					validationCode: peer.TxValidationCode_UNKNOWN_TX_TYPE,
				}
				return
			}

			// Check if there is a duplicate of such transaction in the ledger and
			// obtain the corresponding result that acknowledges the error type
			erroneousResultEntry := v.checkTxIdDupsLedger(tIdx, chdr, v.LedgerResources)
			if erroneousResultEntry != nil {
				results <- erroneousResultEntry
				return
			}
		} else if common.HeaderType(chdr.Type) == common.HeaderType_CONFIG {
			configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
			if err != nil {
				err = errors.WithMessage(err, "error unmarshalling config which passed initial validity checks")
				logger.Criticalf("%+v", err)
				results <- &blockValidationResult{
					tIdx: tIdx,
					err:  err,
				}
				return
			}

			if err := v.ChannelResources.Apply(configEnvelope); err != nil {
				err = errors.WithMessage(err, "error validating config which passed initial validity checks")
				logger.Criticalf("%+v", err)
				results <- &blockValidationResult{
					tIdx: tIdx,
					err:  err,
				}
				return
			}
			logger.Debugf("config transaction received for chain %s", channel)
		} else {
			logger.Warningf("Unknown transaction type [%s] in block number [%d] transaction index [%d]",
				common.HeaderType(chdr.Type), block.Header.Number, tIdx)
			results <- &blockValidationResult{
				tIdx:           tIdx,
				validationCode: peer.TxValidationCode_UNKNOWN_TX_TYPE,
			}
			return
		}

		if _, err := proto.Marshal(env); err != nil {
			logger.Warningf("Cannot marshal transaction: %s", err)
			results <- &blockValidationResult{
				tIdx:           tIdx,
				validationCode: peer.TxValidationCode_MARSHAL_TX_ERROR,
			}
			return
		}
		// Succeeded to pass down here, transaction is valid
		results <- &blockValidationResult{
			tIdx:           tIdx,
			validationCode: peer.TxValidationCode_VALID,
			txid:           txID,
		}
		return
	} else {
		logger.Warning("Nil tx from block")
		results <- &blockValidationResult{
			tIdx:           tIdx,
			validationCode: peer.TxValidationCode_NIL_ENVELOPE,
		}
		return
	}
}

// CheckTxIdDupsLedger returns a vlockValidationResult enhanced with the respective
// error codes if and only if there is transaction with the same transaction identifier
// in the ledger or no decision can be made for whether such transaction exists;
// the function returns nil if it has ensured that there is no such duplicate, such
// that its consumer can proceed with the transaction processing
func (v *TxValidator) checkTxIdDupsLedger(tIdx int, chdr *common.ChannelHeader, ldgr LedgerResources) *blockValidationResult {

	// Retrieve the transaction identifier of the input header
	txID := chdr.TxId

	// Look for a transaction with the same identifier inside the ledger
	_, err := ldgr.GetTransactionByID(txID)

	switch err.(type) {
	case nil:
		// invalid case, returned error is nil. It means that there is already a tx in the ledger with the same id
		logger.Error("Duplicate transaction found, ", txID, ", skipping")
		return &blockValidationResult{
			tIdx:           tIdx,
			validationCode: peer.TxValidationCode_DUPLICATE_TXID,
		}
	case ledger.NotFoundInIndexErr:
		// valid case, returned error is of type NotFoundInIndexErr.
		// It means that no tx with the same id is found in the ledger
		return nil
	default:
		// invalid case, returned error is not of type NotFoundInIndexErr.
		// It means that we could not verify whether a tx with the supplied id is in the ledger
		logger.Errorf("Ledger failure while attempting to detect duplicate status for txid %s: %s", txID, err)
		return &blockValidationResult{
			tIdx: tIdx,
			err:  err,
		}
	}
}

// generateCCKey generates a unique identifier for chaincode in specific channel
func (v *TxValidator) generateCCKey(ccName, chainID string) string {
	return fmt.Sprintf("%s/%s", ccName, chainID)
}

type dynamicDeserializer struct {
	cr ChannelResources
}

func (ds *dynamicDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	return ds.cr.MSPManager().DeserializeIdentity(serializedIdentity)
}

func (ds *dynamicDeserializer) IsWellFormed(identity *mspprotos.SerializedIdentity) error {
	return ds.cr.MSPManager().IsWellFormed(identity)
}

type dynamicCapabilities struct {
	cr ChannelResources
}

func (ds *dynamicCapabilities) ACLs() bool {
	return ds.cr.Capabilities().ACLs()
}

func (ds *dynamicCapabilities) CollectionUpgrade() bool {
	return ds.cr.Capabilities().CollectionUpgrade()
}

// FabToken returns true if fabric token function is supported.
func (ds *dynamicCapabilities) FabToken() bool {
	return ds.cr.Capabilities().FabToken()
}

func (ds *dynamicCapabilities) ForbidDuplicateTXIdInBlock() bool {
	return ds.cr.Capabilities().ForbidDuplicateTXIdInBlock()
}

func (ds *dynamicCapabilities) KeyLevelEndorsement() bool {
	return ds.cr.Capabilities().KeyLevelEndorsement()
}

func (ds *dynamicCapabilities) MetadataLifecycle() bool {
	// This capability no longer exists and should not be referenced in validation anyway
	return false
}

func (ds *dynamicCapabilities) PrivateChannelData() bool {
	return ds.cr.Capabilities().PrivateChannelData()
}

func (ds *dynamicCapabilities) Supported() error {
	return ds.cr.Capabilities().Supported()
}

func (ds *dynamicCapabilities) V1_1Validation() bool {
	return ds.cr.Capabilities().V1_1Validation()
}

func (ds *dynamicCapabilities) V1_2Validation() bool {
	return ds.cr.Capabilities().V1_2Validation()
}

func (ds *dynamicCapabilities) V1_3Validation() bool {
	return ds.cr.Capabilities().V1_3Validation()
}

func (ds *dynamicCapabilities) V2_0Validation() bool {
	return ds.cr.Capabilities().V2_0Validation()
}
