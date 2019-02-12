/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
 */

package plugindispatcher

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	coreUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// ChannelResources provides access to channel artefacts or
// functions to interact with them
type ChannelResources interface {
	// GetMSPIDs returns the IDs for the application MSPs
	// that have been defined in the channel
	GetMSPIDs(cid string) []string
}

// LedgerResources provides access to ledger artefacts or
// functions to interact with them
type LedgerResources interface {
	// NewQueryExecutor gives handle to a query executor.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// LifecycleResources provides access to chaincode lifecycle artefacts or
// functions to interact with them
type LifecycleResources interface {
	// ValidationInfo returns the name and arguments of the validation plugin for the supplied
	// chaincode. The function returns two types of errors, unexpected errors and validation
	// errors. The reason for this is that this function is called from the validation code,
	// which needs to differentiate the two types of error to halt processing on the channel
	// if the unexpected error is not nil and mark the transaction as invalid if the validation
	// error is not nil.
	ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error)
}

//go:generate mockery -dir . -name LifecycleResources -case underscore -output mocks/

var logger = flogging.MustGetLogger("committer.txvalidator")

// dispatcherImpl is the implementation used to call
// the validation plugin and validate block transactions
type dispatcherImpl struct {
	chainID         string
	cr              ChannelResources
	ler             LedgerResources
	lcr             LifecycleResources
	sccprovider     sysccprovider.SystemChaincodeProvider
	pluginValidator *PluginValidator
}

// New creates new plugin dispatcher
func New(chainID string, cr ChannelResources, ler LedgerResources, lcr LifecycleResources, sccp sysccprovider.SystemChaincodeProvider, pluginValidator *PluginValidator) *dispatcherImpl {
	return &dispatcherImpl{
		chainID:         chainID,
		cr:              cr,
		ler:             ler,
		lcr:             lcr,
		sccprovider:     sccp,
		pluginValidator: pluginValidator,
	}
}

// Dispatch executes the validation plugin(s) for transaction
func (v *dispatcherImpl) Dispatch(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (error, peer.TxValidationCode) {
	chainID := v.chainID
	logger.Debugf("[%s] Dispatch starts for bytes %p", chainID, envBytes)

	// get header extensions so we have the chaincode ID
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return err, peer.TxValidationCode_BAD_HEADER_EXTENSION
	}

	// get channel header
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err, peer.TxValidationCode_BAD_CHANNEL_HEADER
	}

	/* obtain the list of namespaces we're writing stuff to;
	   at first, we establish a few facts about this invocation:
	   1) which namespaces does it write to?
	   2) does it write to LSCC's namespace?
	   3) does it write to any cc that cannot be invoked? */
	writesToLSCC := false
	writesToNonInvokableSCC := false
	respPayload, err := utils.GetActionFromEnvelope(envBytes)
	if err != nil {
		return errors.WithMessage(err, "GetActionFromEnvelope failed"), peer.TxValidationCode_BAD_RESPONSE_PAYLOAD
	}
	txRWSet := &rwsetutil.TxRwSet{}
	if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
		return errors.WithMessage(err, "txRWSet.FromProtoBytes failed"), peer.TxValidationCode_BAD_RWSET
	}

	// Verify the header extension and response payload contain the ChaincodeId
	if hdrExt.ChaincodeId == nil {
		return errors.New("nil ChaincodeId in header extension"), peer.TxValidationCode_INVALID_OTHER_REASON
	}

	if respPayload.ChaincodeId == nil {
		return errors.New("nil ChaincodeId in ChaincodeAction"), peer.TxValidationCode_INVALID_OTHER_REASON
	}

	// get name and version of the cc we invoked
	ccID := hdrExt.ChaincodeId.Name
	ccVer := respPayload.ChaincodeId.Version

	// sanity check on ccID
	if ccID == "" {
		err = errors.New("invalid chaincode ID")
		logger.Errorf("%+v", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}
	if ccID != respPayload.ChaincodeId.Name {
		err = errors.Errorf("inconsistent ccid info (%s/%s)", ccID, respPayload.ChaincodeId.Name)
		logger.Errorf("%+v", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}
	// sanity check on ccver
	if ccVer == "" {
		err = errors.New("invalid chaincode version")
		logger.Errorf("%+v", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}

	var wrNamespace []string
	wrNamespace = append(wrNamespace, ccID)
	if respPayload.Events != nil {
		ccEvent := &peer.ChaincodeEvent{}
		if err = proto.Unmarshal(respPayload.Events, ccEvent); err != nil {
			return errors.Wrapf(err, "invalid chaincode event"), peer.TxValidationCode_INVALID_OTHER_REASON
		}
		if ccEvent.ChaincodeId != ccID {
			return errors.Errorf("chaincode event chaincode id does not match chaincode action chaincode id"), peer.TxValidationCode_INVALID_OTHER_REASON
		}
	}

	for _, ns := range txRWSet.NsRwSets {
		if !v.txWritesToNamespace(ns) {
			continue
		}

		// Check to make sure we did not already populate this chaincode
		// name to avoid checking the same namespace twice
		if ns.NameSpace != ccID {
			wrNamespace = append(wrNamespace, ns.NameSpace)
		}

		if !writesToLSCC && ns.NameSpace == "lscc" {
			writesToLSCC = true
		}

		if !writesToNonInvokableSCC && v.sccprovider.IsSysCCAndNotInvokableCC2CC(ns.NameSpace) {
			writesToNonInvokableSCC = true
		}

		if !writesToNonInvokableSCC && v.sccprovider.IsSysCCAndNotInvokableExternal(ns.NameSpace) {
			writesToNonInvokableSCC = true
		}
	}

	// we've gathered all the info required to proceed to validation;
	// validation will behave differently depending on the type of
	// chaincode (system vs. application)

	// 1) we can't write to LSCC - with lifecycle v2.0, LSCC becomes a read-only namespace
	if writesToLSCC {
		// TODO: remove with FAB-13773
		return errors.Errorf("chaincode %s attempted to write to the namespace of LSCC", ccID),
			peer.TxValidationCode_ILLEGAL_WRITESET
	}

	// 2) we can't write to the namespace of a chaincode that we cannot invoke
	if writesToNonInvokableSCC {
		return errors.Errorf("chaincode %s attempted to write to the namespace of a system chaincode that cannot be invoked", ccID),
			peer.TxValidationCode_ILLEGAL_WRITESET
	}

	// validate *EACH* read write set according to its chaincode's endorsement policy
	for _, ns := range wrNamespace {
		// Get latest chaincode validation plugin name and policy
		validationPlugin, policy, err := v.GetInfoForValidate(chdr, ns)
		if err != nil {
			logger.Errorf("GetInfoForValidate for txId = %s returned error: %+v", chdr.TxId, err)
			return err, peer.TxValidationCode_INVALID_OTHER_REASON
		}

		// invoke the plugin
		ctx := &Context{
			Seq:        seq,
			Envelope:   envBytes,
			Block:      block,
			TxID:       chdr.TxId,
			Channel:    chdr.ChannelId,
			Namespace:  ns,
			Policy:     policy,
			PluginName: validationPlugin.ChaincodeName,
		}
		if err = v.invokeValidationPlugin(ctx); err != nil {
			switch err.(type) {
			case *commonerrors.VSCCEndorsementPolicyError:
				return err, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE
			default:
				return err, peer.TxValidationCode_INVALID_OTHER_REASON
			}
		}
	}

	logger.Debugf("[%s] Dispatch completes env bytes %p", chainID, envBytes)
	return nil, peer.TxValidationCode_VALID
}

func (v *dispatcherImpl) invokeValidationPlugin(ctx *Context) error {
	logger.Debug("Validating", ctx, "with plugin")
	err := v.pluginValidator.ValidateWithPlugin(ctx)
	if err == nil {
		return nil
	}
	// If the error is a pluggable validation execution error, cast it to the common errors ExecutionFailureError.
	if e, isExecutionError := err.(*validation.ExecutionFailureError); isExecutionError {
		return &commonerrors.VSCCExecutionFailureError{Err: e}
	}
	// Else, treat it as an endorsement error.
	return &commonerrors.VSCCEndorsementPolicyError{Err: err}
}

func (v *dispatcherImpl) getCDataForCC(channelID, ccid string) (string, []byte, error) {
	qe, err := v.ler.NewQueryExecutor()
	if err != nil {
		return "", nil, errors.WithMessage(err, "could not retrieve QueryExecutor")
	}
	defer qe.Done()

	plugin, args, unexpectedErr, validationErr := v.lcr.ValidationInfo(channelID, ccid, qe)
	if unexpectedErr != nil {
		return "", nil, &commonerrors.VSCCInfoLookupFailureError{
			Reason: fmt.Sprintf("Could not retrieve state for chaincode %s, error %s", ccid, unexpectedErr),
		}
	}
	if validationErr != nil {
		return "", nil, validationErr
	}

	if plugin == "" {
		return "", nil, errors.Errorf("chaincode definition for [%s] is invalid, plugin field must be set", ccid)
	}

	if len(args) == 0 {
		return "", nil, errors.Errorf("chaincode definition for [%s] is invalid, policy field must be set", ccid)
	}

	return plugin, args, nil
}

// GetInfoForValidate gets the ChaincodeInstance(with latest version) of tx, validation plugin and policy
func (v *dispatcherImpl) GetInfoForValidate(chdr *common.ChannelHeader, ccID string) (*sysccprovider.ChaincodeInstance, []byte, error) {
	validationPlugin := &sysccprovider.ChaincodeInstance{
		ChainID:          chdr.ChannelId,
		ChaincodeName:    "vscc",                     // default for system chaincodes
		ChaincodeVersion: coreUtil.GetSysCCVersion(), // Get plugin version
	}
	var policy []byte
	var err error
	if !v.sccprovider.IsSysCC(ccID) {
		// when we are validating a chaincode that is not a
		// system CC, we need to ask the CC to give us the name
		// of the validation plugin and of the policy that should be used

		// obtain name of the validation plugin and the policy
		validationPlugin.ChaincodeName, policy, err = v.getCDataForCC(chdr.ChannelId, ccID)
		if err != nil {
			msg := fmt.Sprintf("Unable to get chaincode data from ledger for txid %s, due to %s", chdr.TxId, err)
			logger.Errorf(msg)
			return nil, nil, err
		}
	} else {
		// when we are validating a system CC, we use the default
		// plugin and a default policy that requires one signature
		// from any of the members of the channel
		p := cauthdsl.SignedByAnyMember(v.cr.GetMSPIDs(chdr.ChannelId))
		policy, err = utils.Marshal(p)
		if err != nil {
			return nil, nil, err
		}
	}

	return validationPlugin, policy, nil
}

// txWritesToNamespace returns true if the supplied NsRwSet
// performs a ledger write
func (v *dispatcherImpl) txWritesToNamespace(ns *rwsetutil.NsRwSet) bool {
	// check for public writes first
	if ns.KvRwSet != nil && len(ns.KvRwSet.Writes) > 0 {
		return true
	}

	// check for private writes for all collections
	for _, c := range ns.CollHashedRwSets {
		if c.HashedRwSet != nil && len(c.HashedRwSet.HashedWrites) > 0 {
			return true
		}

		// private metadata updates
		if c.HashedRwSet != nil && len(c.HashedRwSet.MetadataWrites) > 0 {
			return true
		}
	}

	if ns.KvRwSet != nil && len(ns.KvRwSet.MetadataWrites) > 0 {
		return true
	}

	return false
}
