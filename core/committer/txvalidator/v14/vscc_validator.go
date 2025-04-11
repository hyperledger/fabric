/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// VsccValidatorImpl is the implementation used to call
// the vscc chaincode and validate block transactions
type VsccValidatorImpl struct {
	channelID       string
	cr              ChannelResources
	pluginValidator *PluginValidator
}

// newVSCCValidator creates new vscc validator
func newVSCCValidator(channelID string, cr ChannelResources, pluginValidator *PluginValidator) *VsccValidatorImpl {
	return &VsccValidatorImpl{
		channelID:       channelID,
		cr:              cr,
		pluginValidator: pluginValidator,
	}
}

func getChaincodeHeaderExtension(hdr *common.Header) (*peer.ChaincodeHeaderExtension, error) {
	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	chaincodeHdrExt := &peer.ChaincodeHeaderExtension{}
	err = proto.Unmarshal(chdr.Extension, chaincodeHdrExt)
	return chaincodeHdrExt, errors.Wrap(err, "error unmarshalling ChaincodeHeaderExtension")
}

// VSCCValidateTx executes vscc validation for transaction
func (v *VsccValidatorImpl) VSCCValidateTx(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (peer.TxValidationCode, error) {
	chainID := v.channelID
	logger.Debugf("[%s] VSCCValidateTx starts for bytes %p", chainID, envBytes)

	// get header extensions so we have the chaincode ID
	hdrExt, err := getChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return peer.TxValidationCode_BAD_HEADER_EXTENSION, err
	}

	// get channel header
	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return peer.TxValidationCode_BAD_CHANNEL_HEADER, err
	}

	/* obtain the list of namespaces we're writing stuff to;
	   at first, we establish a few facts about this invocation:
	   1) which namespaces does it write to?
	   2) does it write to LSCC's namespace?
	   3) does it write to any cc that cannot be invoked? */
	writesToLSCC := false
	writesToNonInvokableSCC := false
	respPayload, err := protoutil.GetActionFromEnvelope(envBytes)
	if err != nil {
		return peer.TxValidationCode_BAD_RESPONSE_PAYLOAD, errors.WithMessage(err, "GetActionFromEnvelope failed")
	}
	txRWSet := &rwsetutil.TxRwSet{}
	if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
		return peer.TxValidationCode_BAD_RWSET, errors.WithMessage(err, "txRWSet.FromProtoBytes failed")
	}

	// Verify the header extension and response payload contain the ChaincodeId
	if hdrExt.ChaincodeId == nil {
		return peer.TxValidationCode_INVALID_OTHER_REASON, errors.New("nil ChaincodeId in header extension")
	}

	if respPayload.ChaincodeId == nil {
		return peer.TxValidationCode_INVALID_OTHER_REASON, errors.New("nil ChaincodeId in ChaincodeAction")
	}

	// get name and version of the cc we invoked
	ccID := hdrExt.ChaincodeId.Name
	ccVer := respPayload.ChaincodeId.Version

	// sanity check on ccID
	if ccID == "" {
		err = errors.New("invalid chaincode ID")
		logger.Errorf("%+v", err)
		return peer.TxValidationCode_INVALID_OTHER_REASON, err
	}
	if ccID != respPayload.ChaincodeId.Name {
		err = errors.Errorf("inconsistent ccid info (%s/%s)", ccID, respPayload.ChaincodeId.Name)
		logger.Errorf("%+v", err)
		return peer.TxValidationCode_INVALID_OTHER_REASON, err
	}
	// sanity check on ccver
	if ccVer == "" {
		err = errors.New("invalid chaincode version")
		logger.Errorf("%+v", err)
		return peer.TxValidationCode_INVALID_OTHER_REASON, err
	}

	var wrNamespace []string
	alwaysEnforceOriginalNamespace := v.cr.Capabilities().V1_2Validation()
	if alwaysEnforceOriginalNamespace {
		wrNamespace = append(wrNamespace, ccID)
		if respPayload.Events != nil {
			ccEvent := &peer.ChaincodeEvent{}
			if err = proto.Unmarshal(respPayload.Events, ccEvent); err != nil {
				return peer.TxValidationCode_INVALID_OTHER_REASON, errors.Wrapf(err, "invalid chaincode event")
			}
			if ccEvent.ChaincodeId != ccID {
				return peer.TxValidationCode_INVALID_OTHER_REASON, errors.Errorf("chaincode event chaincode id does not match chaincode action chaincode id")
			}
		}
	}

	namespaces := make(map[string]struct{})
	for _, ns := range txRWSet.NsRwSets {
		// check to make sure there is no duplicate namespace in txRWSet
		if _, ok := namespaces[ns.NameSpace]; ok {
			return peer.TxValidationCode_ILLEGAL_WRITESET, errors.Errorf("duplicate namespace '%s' in txRWSet", ns.NameSpace)
		}
		namespaces[ns.NameSpace] = struct{}{}

		if !v.txWritesToNamespace(ns) {
			continue
		}

		// Check to make sure we did not already populate this chaincode
		// name to avoid checking the same namespace twice
		if ns.NameSpace != ccID || !alwaysEnforceOriginalNamespace {
			wrNamespace = append(wrNamespace, ns.NameSpace)
		}

		if !writesToLSCC && ns.NameSpace == "lscc" {
			writesToLSCC = true
		}

		if !writesToNonInvokableSCC && IsSysCCAndNotInvokableCC2CC(ns.NameSpace) {
			writesToNonInvokableSCC = true
		}

		if !writesToNonInvokableSCC && IsSysCCAndNotInvokableExternal(ns.NameSpace) {
			writesToNonInvokableSCC = true
		}
	}

	// we've gathered all the info required to proceed to validation;
	// validation will behave differently depending on the type of
	// chaincode (system vs. application)

	if !IsSysCC(ccID) {
		// if we're here, we know this is an invocation of an application chaincode;
		// first of all, we make sure that:
		// 1) we don't write to LSCC - an application chaincode is free to invoke LSCC
		//    for instance to get information about itself or another chaincode; however
		//    these legitimate invocations only ready from LSCC's namespace; currently
		//    only two functions of LSCC write to its namespace: deploy and upgrade and
		//    neither should be used by an application chaincode
		if writesToLSCC {
			return peer.TxValidationCode_ILLEGAL_WRITESET,
				errors.Errorf("chaincode %s attempted to write to the namespace of LSCC", ccID)
		}
		// 2) we don't write to the namespace of a chaincode that we cannot invoke - if
		//    the chaincode cannot be invoked in the first place, there's no legitimate
		//    way in which a transaction has a write set that writes to it; additionally
		//    we don't have any means of verifying whether the transaction had the rights
		//    to perform that write operation because in v1, system chaincodes do not have
		//    any endorsement policies to speak of. So if the chaincode can't be invoked
		//    it can't be written to by an invocation of an application chaincode
		if writesToNonInvokableSCC {
			return peer.TxValidationCode_ILLEGAL_WRITESET,
				errors.Errorf("chaincode %s attempted to write to the namespace of a system chaincode that cannot be invoked", ccID)
		}

		// validate *EACH* read write set according to its chaincode's endorsement policy
		for _, ns := range wrNamespace {
			// Get latest chaincode version, vscc and validate policy
			txcc, vscc, policy, err := v.GetInfoForValidate(chdr, ns)
			if err != nil {
				logger.Errorf("GetInfoForValidate for txId = %s returned error: %+v", chdr.TxId, err)
				return peer.TxValidationCode_INVALID_OTHER_REASON, err
			}

			// if the namespace corresponds to the cc that was originally
			// invoked, we check that the version of the cc that was
			// invoked corresponds to the version that lscc has returned
			if ns == ccID && txcc.ChaincodeVersion != ccVer {
				err = errors.Errorf("chaincode %s:%s/%s didn't match %s:%s/%s in lscc", ccID, ccVer, chdr.ChannelId, txcc.ChaincodeName, txcc.ChaincodeVersion, chdr.ChannelId)
				logger.Errorf("%+v", err)
				return peer.TxValidationCode_EXPIRED_CHAINCODE, err
			}

			// do VSCC validation
			ctx := &Context{
				Seq:       seq,
				Envelope:  envBytes,
				Block:     block,
				TxID:      chdr.TxId,
				Channel:   chdr.ChannelId,
				Namespace: ns,
				Policy:    policy,
				VSCCName:  vscc.ChaincodeName,
			}
			if err = v.VSCCValidateTxForCC(ctx); err != nil {
				switch err.(type) {
				case *commonerrors.VSCCEndorsementPolicyError:
					return peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE, err
				default:
					return peer.TxValidationCode_INVALID_OTHER_REASON, err
				}
			}
		}
	} else {
		// make sure that we can invoke this system chaincode - if the chaincode
		// cannot be invoked through a proposal to this peer, we have to drop the
		// transaction; if we didn't, we wouldn't know how to decide whether it's
		// valid or not because in v1, system chaincodes have no endorsement policy
		if IsSysCCAndNotInvokableExternal(ccID) {
			return peer.TxValidationCode_ILLEGAL_WRITESET,
				errors.Errorf("committing an invocation of cc %s is illegal", ccID)
		}

		// Get latest chaincode version, vscc and validate policy
		_, vscc, policy, err := v.GetInfoForValidate(chdr, ccID)
		if err != nil {
			logger.Errorf("GetInfoForValidate for txId = %s returned error: %+v", chdr.TxId, err)
			return peer.TxValidationCode_INVALID_OTHER_REASON, err
		}

		// validate the transaction as an invocation of this system chaincode;
		// vscc will have to do custom validation for this system chaincode
		// currently, VSCC does custom validation for LSCC only; if an hlf
		// user creates a new system chaincode which is invokable from the outside
		// they have to modify VSCC to provide appropriate validation
		ctx := &Context{
			Seq:       seq,
			Envelope:  envBytes,
			Block:     block,
			TxID:      chdr.TxId,
			Channel:   chdr.ChannelId,
			Namespace: ccID,
			Policy:    policy,
			VSCCName:  vscc.ChaincodeName,
		}
		if err = v.VSCCValidateTxForCC(ctx); err != nil {
			switch err.(type) {
			case *commonerrors.VSCCEndorsementPolicyError:
				return peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE, err
			default:
				return peer.TxValidationCode_INVALID_OTHER_REASON, err
			}
		}
	}
	logger.Debugf("[%s] VSCCValidateTx completes env bytes %p", chainID, envBytes)
	return peer.TxValidationCode_VALID, nil
}

func (v *VsccValidatorImpl) VSCCValidateTxForCC(ctx *Context) error {
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

func (v *VsccValidatorImpl) getCDataForCC(chid, ccid string) (*ccprovider.ChaincodeData, error) {
	l := v.cr.Ledger()
	if l == nil {
		return nil, errors.New("nil ledger instance")
	}

	qe, err := l.NewQueryExecutor()
	if err != nil {
		return nil, errors.WithMessage(err, "could not retrieve QueryExecutor")
	}
	defer qe.Done()

	bytes, err := qe.GetState("lscc", ccid)
	if err != nil {
		return nil, &commonerrors.VSCCInfoLookupFailureError{
			Reason: fmt.Sprintf("Could not retrieve state for chaincode %s, error %s", ccid, err),
		}
	}

	if bytes == nil {
		return nil, errors.Errorf("lscc's state for [%s] not found.", ccid)
	}

	cd := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(bytes, cd)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling ChaincodeQueryResponse failed")
	}

	if cd.Vscc == "" {
		return nil, errors.Errorf("lscc's state for [%s] is invalid, vscc field must be set", ccid)
	}

	if len(cd.Policy) == 0 {
		return nil, errors.Errorf("lscc's state for [%s] is invalid, policy field must be set", ccid)
	}

	return cd, err
}

// GetInfoForValidate gets the ChaincodeInstance(with latest version) of tx, vscc and policy from lscc
func (v *VsccValidatorImpl) GetInfoForValidate(chdr *common.ChannelHeader, ccID string) (*sysccprovider.ChaincodeInstance, *sysccprovider.ChaincodeInstance, []byte, error) {
	cc := &sysccprovider.ChaincodeInstance{
		ChannelID:     chdr.ChannelId,
		ChaincodeName: ccID,
	}
	vscc := &sysccprovider.ChaincodeInstance{
		ChannelID:     chdr.ChannelId,
		ChaincodeName: "vscc", // default vscc for system chaincodes
	}
	var policy []byte
	var err error
	if !IsSysCC(ccID) {
		// when we are validating a chaincode that is not a
		// system CC, we need to ask the CC to give us the name
		// of VSCC and of the policy that should be used

		// obtain name of the VSCC and the policy
		cd, err := v.getCDataForCC(chdr.ChannelId, ccID)
		if err != nil {
			logger.Errorf("Unable to get chaincode data from ledger for txid %s, due to %s", chdr.TxId, err)
			return nil, nil, nil, err
		}
		cc.ChaincodeName = cd.Name
		cc.ChaincodeVersion = cd.Version
		vscc.ChaincodeName, policy = cd.Vscc, cd.Policy
	} else {
		// when we are validating a system CC, we use the default
		// VSCC and a default policy that requires one signature
		// from any of the members of the channel
		p := policydsl.SignedByAnyMember(v.cr.GetMSPIDs())
		policy, err = protoutil.Marshal(p)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return cc, vscc, policy, nil
}

// txWritesToNamespace returns true if the supplied NsRwSet
// performs a ledger write
func (v *VsccValidatorImpl) txWritesToNamespace(ns *rwsetutil.NsRwSet) bool {
	// check for public writes first
	if ns.KvRwSet != nil && len(ns.KvRwSet.Writes) > 0 {
		return true
	}

	// only look at collection data if we support that capability
	if v.cr.Capabilities().PrivateChannelData() {
		// check for private writes for all collections
		for _, c := range ns.CollHashedRwSets {
			if c.HashedRwSet != nil && len(c.HashedRwSet.HashedWrites) > 0 {
				return true
			}

			// only look at private metadata writes if we support that capability
			if v.cr.Capabilities().KeyLevelEndorsement() {
				// private metadata updates
				if c.HashedRwSet != nil && len(c.HashedRwSet.MetadataWrites) > 0 {
					return true
				}
			}
		}
	}

	// only look at metadata writes if we support that capability
	if v.cr.Capabilities().KeyLevelEndorsement() {
		// public metadata updates
		if ns.KvRwSet != nil && len(ns.KvRwSet.MetadataWrites) > 0 {
			return true
		}
	}

	return false
}

func IsSysCCAndNotInvokableExternal(name string) bool {
	return name == "vscc" || name == "escc"
}

func IsSysCC(name string) bool {
	return name == "vscc" || name == "escc" || name == "lscc" || name == "qscc" || name == "cscc"
}

func IsSysCCAndNotInvokableCC2CC(name string) bool {
	return name == "vscc" || name == "escc" || name == "cscc"
}
