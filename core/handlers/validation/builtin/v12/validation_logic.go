/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v12

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	vc "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vi "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	vp "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var (
	logger = flogging.MustGetLogger("vscc")

	// currently defined system chaincode names that shouldn't
	// be allowed as user-defined chaincode names
	systemChaincodeNames = map[string]struct{}{
		"cscc": {},
		"escc": {},
		"lscc": {},
		"qscc": {},
		"vscc": {},
	}
)

const (
	DUPLICATED_IDENTITY_ERROR = "Endorsement policy evaluation failure might be caused by duplicated identities"
)

const AllowedCharsCollectionName = "[A-Za-z0-9_-]+"

var validCollectionNameRegex = regexp.MustCompile(AllowedCharsCollectionName)

//go:generate mockery -dir . -name Capabilities -case underscore -output mocks/

// Capabilities is the local interface that used to generate mocks for foreign interface.
type Capabilities interface {
	vc.Capabilities
}

//go:generate mockery -dir . -name StateFetcher -case underscore -output mocks/

// StateFetcher is the local interface that used to generate mocks for foreign interface.
type StateFetcher interface {
	vs.StateFetcher
}

//go:generate mockery -dir . -name IdentityDeserializer -case underscore -output mocks/

// IdentityDeserializer is the local interface that used to generate mocks for foreign interface.
type IdentityDeserializer interface {
	vi.IdentityDeserializer
}

//go:generate mockery -dir . -name PolicyEvaluator -case underscore -output mocks/

// PolicyEvaluator is the local interface that used to generate mocks for foreign interface.
type PolicyEvaluator interface {
	vp.PolicyEvaluator
}

// New creates a new instance of the default VSCC
// Typically this will only be invoked once per peer.
func New(c vc.Capabilities, s vs.StateFetcher, d vi.IdentityDeserializer, pe vp.PolicyEvaluator) *Validator {
	return &Validator{
		capabilities:    c,
		stateFetcher:    s,
		deserializer:    d,
		policyEvaluator: pe,
	}
}

// Validator implements the default transaction validation policy,
// which is to check the correctness of the read-write set and the endorsement
// signatures against an endorsement policy that is supplied as argument to
// every invoke.
type Validator struct {
	deserializer    vi.IdentityDeserializer
	capabilities    vc.Capabilities
	stateFetcher    vs.StateFetcher
	policyEvaluator vp.PolicyEvaluator
}

// Validate validates the given envelope corresponding to a transaction with an endorsement
// policy as given in its serialized form.
func (vscc *Validator) Validate(
	block *common.Block,
	namespace string,
	txPosition int,
	actionPosition int,
	policyBytes []byte,
) commonerrors.TxValidationError {
	// get the envelope...
	env, err := protoutil.GetEnvelopeFromBlock(block.Data.Data[txPosition])
	if err != nil {
		logger.Errorf("VSCC error: GetEnvelope failed, err %s", err)
		return policyErr(err)
	}

	// ...and the payload...
	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return policyErr(err)
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return policyErr(err)
	}

	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type)
		return policyErr(fmt.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type))
	}

	// ...and the transaction...
	tx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return policyErr(err)
	}

	cap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[actionPosition].Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
		return policyErr(err)
	}

	signatureSet, err := vscc.deduplicateIdentity(cap)
	if err != nil {
		return policyErr(err)
	}

	// evaluate the signature set against the policy
	err = vscc.policyEvaluator.Evaluate(policyBytes, signatureSet)
	if err != nil {
		logger.Warningf("Endorsement policy failure for transaction txid=%s, err: %s", chdr.GetTxId(), err.Error())
		if len(signatureSet) < len(cap.Action.Endorsements) {
			// Warning: duplicated identities exist, endorsement failure might be cause by this reason
			return policyErr(errors.New(DUPLICATED_IDENTITY_ERROR))
		}
		return policyErr(fmt.Errorf("VSCC error: endorsement policy failure, err: %s", err))
	}

	// do some extra validation that is specific to lscc
	if namespace == "lscc" {
		logger.Debugf("VSCC info: doing special validation for LSCC")
		err := vscc.ValidateLSCCInvocation(chdr.ChannelId, env, cap, payl, vscc.capabilities)
		if err != nil {
			logger.Errorf("VSCC error: ValidateLSCCInvocation failed, err %s", err)
			return err
		}
	}

	return nil
}

// checkInstantiationPolicy evaluates an instantiation policy against a signed proposal.
func (vscc *Validator) checkInstantiationPolicy(chainName string, env *common.Envelope, instantiationPolicy []byte, payl *common.Payload) commonerrors.TxValidationError {
	// get the signature header
	shdr, err := protoutil.UnmarshalSignatureHeader(payl.Header.SignatureHeader)
	if err != nil {
		return policyErr(err)
	}

	// construct signed data we can evaluate the instantiation policy against
	sd := []*protoutil.SignedData{{
		Data:      env.Payload,
		Identity:  shdr.Creator,
		Signature: env.Signature,
	}}
	err = vscc.policyEvaluator.Evaluate(instantiationPolicy, sd)
	if err != nil {
		return policyErr(fmt.Errorf("chaincode instantiation policy violated, error %s", err))
	}
	return nil
}

func validateNewCollectionConfigs(newCollectionConfigs []*pb.CollectionConfig) error {
	newCollectionsMap := make(map[string]bool, len(newCollectionConfigs))
	// Process each collection config from a set of collection configs
	for _, newCollectionConfig := range newCollectionConfigs {

		newCollection := newCollectionConfig.GetStaticCollectionConfig()
		if newCollection == nil {
			return errors.New("unknown collection configuration type")
		}

		// Ensure that there are no duplicate collection names
		collectionName := newCollection.GetName()

		if err := validateCollectionName(collectionName); err != nil {
			return err
		}

		if _, ok := newCollectionsMap[collectionName]; !ok {
			newCollectionsMap[collectionName] = true
		} else {
			return fmt.Errorf("collection-name: %s -- found duplicate collection configuration", collectionName)
		}

		// Validate gossip related parameters present in the collection config
		maximumPeerCount := newCollection.GetMaximumPeerCount()
		requiredPeerCount := newCollection.GetRequiredPeerCount()
		if maximumPeerCount < requiredPeerCount {
			return fmt.Errorf("collection-name: %s -- maximum peer count (%d) cannot be less than the required peer count (%d)",
				collectionName, maximumPeerCount, requiredPeerCount)
		}
		if requiredPeerCount < 0 {
			return fmt.Errorf("collection-name: %s -- requiredPeerCount (%d) cannot be less than zero",
				collectionName, requiredPeerCount)
		}

		// make sure that the signature policy is meaningful (only consists of ORs)
		err := validateSpOrConcat(newCollection.MemberOrgsPolicy.GetSignaturePolicy().Rule)
		if err != nil {
			return errors.WithMessagef(err, "collection-name: %s -- error in member org policy", collectionName)
		}
	}
	return nil
}

// validateSpOrConcat checks if the supplied signature policy is just an OR-concatenation of identities.
func validateSpOrConcat(sp *common.SignaturePolicy) error {
	if sp.GetNOutOf() == nil {
		return nil
	}
	// check if N == 1 (OR concatenation)
	if sp.GetNOutOf().N != 1 {
		return errors.New(fmt.Sprintf("signature policy is not an OR concatenation, NOutOf %d", sp.GetNOutOf().N))
	}
	// recurse into all sub-rules
	for _, rule := range sp.GetNOutOf().Rules {
		err := validateSpOrConcat(rule)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkForMissingCollections(newCollectionsMap map[string]*pb.StaticCollectionConfig, oldCollectionConfigs []*pb.CollectionConfig,
) error {
	var missingCollections []string

	// In the new collection config package, ensure that there is one entry per old collection. Any
	// number of new collections are allowed.
	for _, oldCollectionConfig := range oldCollectionConfigs {

		oldCollection := oldCollectionConfig.GetStaticCollectionConfig()
		// It cannot be nil
		if oldCollection == nil {
			return policyErr(fmt.Errorf("unknown collection configuration type"))
		}

		// All old collection must exist in the new collection config package
		oldCollectionName := oldCollection.GetName()
		_, ok := newCollectionsMap[oldCollectionName]
		if !ok {
			missingCollections = append(missingCollections, oldCollectionName)
		}
	}

	if len(missingCollections) > 0 {
		return policyErr(fmt.Errorf("the following existing collections are missing in the new collection configuration package: %v",
			missingCollections))
	}

	return nil
}

func checkForModifiedCollectionsBTL(newCollectionsMap map[string]*pb.StaticCollectionConfig, oldCollectionConfigs []*pb.CollectionConfig,
) error {
	var modifiedCollectionsBTL []string

	// In the new collection config package, ensure that the block to live value is not
	// modified for the existing collections.
	for _, oldCollectionConfig := range oldCollectionConfigs {

		oldCollection := oldCollectionConfig.GetStaticCollectionConfig()
		// It cannot be nil
		if oldCollection == nil {
			return policyErr(fmt.Errorf("unknown collection configuration type"))
		}

		oldCollectionName := oldCollection.GetName()
		newCollection := newCollectionsMap[oldCollectionName]
		// BlockToLive cannot be changed
		if newCollection.GetBlockToLive() != oldCollection.GetBlockToLive() {
			modifiedCollectionsBTL = append(modifiedCollectionsBTL, oldCollectionName)
		}
	}

	if len(modifiedCollectionsBTL) > 0 {
		return policyErr(fmt.Errorf("the BlockToLive in the following existing collections must not be modified: %v",
			modifiedCollectionsBTL))
	}

	return nil
}

func validateNewCollectionConfigsAgainstOld(newCollectionConfigs []*pb.CollectionConfig, oldCollectionConfigs []*pb.CollectionConfig,
) error {
	newCollectionsMap := make(map[string]*pb.StaticCollectionConfig, len(newCollectionConfigs))

	for _, newCollectionConfig := range newCollectionConfigs {
		newCollection := newCollectionConfig.GetStaticCollectionConfig()
		// Collection object itself is stored as value so that we can
		// check whether the block to live is changed -- FAB-7810
		newCollectionsMap[newCollection.GetName()] = newCollection
	}

	if err := checkForMissingCollections(newCollectionsMap, oldCollectionConfigs); err != nil {
		return err
	}

	if err := checkForModifiedCollectionsBTL(newCollectionsMap, oldCollectionConfigs); err != nil {
		return err
	}

	return nil
}

func validateCollectionName(collectionName string) error {
	if collectionName == "" {
		return fmt.Errorf("empty collection-name is not allowed")
	}
	match := validCollectionNameRegex.FindString(collectionName)
	if len(match) != len(collectionName) {
		return fmt.Errorf("collection-name: %s not allowed. A valid collection name follows the pattern: %s",
			collectionName, AllowedCharsCollectionName)
	}
	return nil
}

// validateRWSetAndCollection performs validation of the rwset
// of an LSCC deploy operation and then it validates any collection
// configuration.
func (vscc *Validator) validateRWSetAndCollection(
	lsccrwset *kvrwset.KVRWSet,
	cdRWSet *ccprovider.ChaincodeData,
	lsccArgs [][]byte,
	lsccFunc string,
	ac vc.Capabilities,
	channelName string,
) commonerrors.TxValidationError {
	/********************************************/
	/* security check 0.a - validation of rwset */
	/********************************************/
	// there can only be one or two writes
	if len(lsccrwset.Writes) > 2 {
		return policyErr(fmt.Errorf("LSCC can only issue one or two putState upon deploy"))
	}

	/**********************************************************/
	/* security check 0.b - validation of the collection data */
	/**********************************************************/
	var collectionsConfigArg []byte
	if len(lsccArgs) > 5 {
		collectionsConfigArg = lsccArgs[5]
	}

	var collectionsConfigLedger []byte
	if len(lsccrwset.Writes) == 2 {
		key := privdata.BuildCollectionKVSKey(cdRWSet.Name)
		if lsccrwset.Writes[1].Key != key {
			return policyErr(fmt.Errorf("invalid key for the collection of chaincode %s:%s; expected '%s', received '%s'",
				cdRWSet.Name, cdRWSet.Version, key, lsccrwset.Writes[1].Key))
		}

		collectionsConfigLedger = lsccrwset.Writes[1].Value
	}

	if !bytes.Equal(collectionsConfigArg, collectionsConfigLedger) {
		return policyErr(fmt.Errorf("collection configuration arguments supplied for chaincode %s:%s do not match the configuration in the lscc writeset",
			cdRWSet.Name, cdRWSet.Version))
	}

	channelState, err := vscc.stateFetcher.FetchState()
	if err != nil {
		return &commonerrors.VSCCExecutionFailureError{Err: fmt.Errorf("failed obtaining query executor: %v", err)}
	}
	defer channelState.Done()

	state := &state{channelState}

	// The following condition check added in v1.1 may not be needed as it is not possible to have the chaincodeName~collection key in
	// the lscc namespace before a chaincode deploy. To avoid forks in v1.2, the following condition is retained.
	if lsccFunc == lscc.DEPLOY {
		colCriteria := privdata.CollectionCriteria{Channel: channelName, Namespace: cdRWSet.Name}
		ccp, err := privdata.RetrieveCollectionConfigPackageFromState(colCriteria, state)
		if err != nil {
			// fail if we get any error other than NoSuchCollectionError
			// because it means something went wrong while looking up the
			// older collection
			if _, ok := err.(privdata.NoSuchCollectionError); !ok {
				return &commonerrors.VSCCExecutionFailureError{
					Err: fmt.Errorf("unable to check whether collection existed earlier for chaincode %s:%s",
						cdRWSet.Name, cdRWSet.Version),
				}
			}
		}
		if ccp != nil {
			return policyErr(fmt.Errorf("collection data should not exist for chaincode %s:%s", cdRWSet.Name, cdRWSet.Version))
		}
	}

	// TODO: Once the new chaincode lifecycle is available (FAB-8724), the following validation
	// and other validation performed in ValidateLSCCInvocation can be moved to LSCC itself.
	newCollectionConfigPackage := &pb.CollectionConfigPackage{}

	if collectionsConfigArg != nil {
		err := proto.Unmarshal(collectionsConfigArg, newCollectionConfigPackage)
		if err != nil {
			return policyErr(fmt.Errorf("invalid collection configuration supplied for chaincode %s:%s",
				cdRWSet.Name, cdRWSet.Version))
		}
	} else {
		return nil
	}

	if ac.V1_2Validation() {
		newCollectionConfigs := newCollectionConfigPackage.GetConfig()
		if err := validateNewCollectionConfigs(newCollectionConfigs); err != nil {
			return policyErr(err)
		}

		if lsccFunc == lscc.UPGRADE {

			collectionCriteria := privdata.CollectionCriteria{Channel: channelName, Namespace: cdRWSet.Name}
			// oldCollectionConfigPackage denotes the existing collection config package in the ledger
			oldCollectionConfigPackage, err := privdata.RetrieveCollectionConfigPackageFromState(collectionCriteria, state)
			if err != nil {
				// fail if we get any error other than NoSuchCollectionError
				// because it means something went wrong while looking up the
				// older collection
				if _, ok := err.(privdata.NoSuchCollectionError); !ok {
					return &commonerrors.VSCCExecutionFailureError{
						Err: fmt.Errorf("unable to check whether collection existed earlier for chaincode %s:%s: %v",
							cdRWSet.Name, cdRWSet.Version, err),
					}
				}
			}

			// oldCollectionConfigPackage denotes the existing collection config package in the ledger
			if oldCollectionConfigPackage != nil {
				oldCollectionConfigs := oldCollectionConfigPackage.GetConfig()
				if err := validateNewCollectionConfigsAgainstOld(newCollectionConfigs, oldCollectionConfigs); err != nil {
					return policyErr(err)
				}

			}
		}
	}

	return nil
}

func (vscc *Validator) ValidateLSCCInvocation(
	chid string,
	env *common.Envelope,
	cap *pb.ChaincodeActionPayload,
	payl *common.Payload,
	ac vc.Capabilities,
) commonerrors.TxValidationError {
	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(cap.ChaincodeProposalPayload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeProposalPayload failed, err %s", err)
		return policyErr(err)
	}

	cis := &pb.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(cpp.Input, cis)
	if err != nil {
		logger.Errorf("VSCC error: Unmarshal ChaincodeInvocationSpec failed, err %s", err)
		return policyErr(err)
	}

	if cis.ChaincodeSpec == nil ||
		cis.ChaincodeSpec.Input == nil ||
		cis.ChaincodeSpec.Input.Args == nil {
		logger.Errorf("VSCC error: committing invalid vscc invocation")
		return policyErr(fmt.Errorf("malformed chaincode invocation spec"))
	}

	lsccFunc := string(cis.ChaincodeSpec.Input.Args[0])
	lsccArgs := cis.ChaincodeSpec.Input.Args[1:]

	logger.Debugf("VSCC info: ValidateLSCCInvocation acting on %s %#v", lsccFunc, lsccArgs)

	switch lsccFunc {
	case lscc.UPGRADE, lscc.DEPLOY:
		logger.Debugf("VSCC info: validating invocation of lscc function %s on arguments %#v", lsccFunc, lsccArgs)

		if len(lsccArgs) < 2 {
			return policyErr(fmt.Errorf("Wrong number of arguments for invocation lscc(%s): expected at least 2, received %d", lsccFunc, len(lsccArgs)))
		}

		if (!ac.PrivateChannelData() && len(lsccArgs) > 5) ||
			(ac.PrivateChannelData() && len(lsccArgs) > 6) {
			return policyErr(fmt.Errorf("Wrong number of arguments for invocation lscc(%s): received %d", lsccFunc, len(lsccArgs)))
		}

		cdsArgs, err := protoutil.UnmarshalChaincodeDeploymentSpec(lsccArgs[1])
		if err != nil {
			return policyErr(fmt.Errorf("GetChaincodeDeploymentSpec error %s", err))
		}

		if cdsArgs == nil || cdsArgs.ChaincodeSpec == nil || cdsArgs.ChaincodeSpec.ChaincodeId == nil ||
			cap.Action == nil || cap.Action.ProposalResponsePayload == nil {
			return policyErr(fmt.Errorf("VSCC error: invocation of lscc(%s) does not have appropriate arguments", lsccFunc))
		}

		switch cdsArgs.ChaincodeSpec.Type.String() {
		case "GOLANG", "NODE", "JAVA", "CAR":
		default:
			return policyErr(fmt.Errorf("unexpected chaincode spec type: %s", cdsArgs.ChaincodeSpec.Type.String()))
		}

		// validate chaincode name
		ccName := cdsArgs.ChaincodeSpec.ChaincodeId.Name
		// it must comply with the lscc.ChaincodeNameRegExp
		if !lscc.ChaincodeNameRegExp.MatchString(ccName) {
			return policyErr(errors.Errorf("invalid chaincode name '%s'", ccName))
		}
		// it can't match the name of one of the system chaincodes
		if _, in := systemChaincodeNames[ccName]; in {
			return policyErr(errors.Errorf("chaincode name '%s' is reserved for system chaincodes", ccName))
		}

		// validate chaincode version
		ccVersion := cdsArgs.ChaincodeSpec.ChaincodeId.Version
		// it must comply with the lscc.ChaincodeVersionRegExp
		if !lscc.ChaincodeVersionRegExp.MatchString(ccVersion) {
			return policyErr(errors.Errorf("invalid chaincode version '%s'", ccVersion))
		}

		// get the rwset
		pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(cap.Action.ProposalResponsePayload)
		if err != nil {
			return policyErr(fmt.Errorf("GetProposalResponsePayload error %s", err))
		}
		if pRespPayload.Extension == nil {
			return policyErr(fmt.Errorf("nil pRespPayload.Extension"))
		}
		respPayload, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
		if err != nil {
			return policyErr(fmt.Errorf("GetChaincodeAction error %s", err))
		}
		txRWSet := &rwsetutil.TxRwSet{}
		if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
			return policyErr(fmt.Errorf("txRWSet.FromProtoBytes error %s", err))
		}

		// extract the rwset for lscc
		var lsccrwset *kvrwset.KVRWSet
		for _, ns := range txRWSet.NsRwSets {
			logger.Debugf("Namespace %s", ns.NameSpace)
			if ns.NameSpace == "lscc" {
				lsccrwset = ns.KvRwSet
				break
			}
		}

		// retrieve from the ledger the entry for the chaincode at hand
		cdLedger, ccExistsOnLedger, err := vscc.getInstantiatedCC(chid, cdsArgs.ChaincodeSpec.ChaincodeId.Name)
		if err != nil {
			return &commonerrors.VSCCExecutionFailureError{Err: err}
		}

		/******************************************/
		/* security check 0 - validation of rwset */
		/******************************************/
		// there has to be a write-set
		if lsccrwset == nil {
			return policyErr(fmt.Errorf("No read write set for lscc was found"))
		}
		// there must be at least one write
		if len(lsccrwset.Writes) < 1 {
			return policyErr(fmt.Errorf("LSCC must issue at least one single putState upon deploy/upgrade"))
		}
		// the first key name must be the chaincode id provided in the deployment spec
		if lsccrwset.Writes[0].Key != cdsArgs.ChaincodeSpec.ChaincodeId.Name {
			return policyErr(fmt.Errorf("expected key %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name, lsccrwset.Writes[0].Key))
		}
		// the value must be a ChaincodeData struct
		cdRWSet := &ccprovider.ChaincodeData{}
		err = proto.Unmarshal(lsccrwset.Writes[0].Value, cdRWSet)
		if err != nil {
			return policyErr(fmt.Errorf("unmarshalling of ChaincodeData failed, error %s", err))
		}
		// the chaincode name in the lsccwriteset must match the chaincode name in the deployment spec
		if cdRWSet.Name != cdsArgs.ChaincodeSpec.ChaincodeId.Name {
			return policyErr(fmt.Errorf("expected cc name %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name, cdRWSet.Name))
		}
		// the chaincode version in the lsccwriteset must match the chaincode version in the deployment spec
		if cdRWSet.Version != cdsArgs.ChaincodeSpec.ChaincodeId.Version {
			return policyErr(fmt.Errorf("expected cc version %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Version, cdRWSet.Version))
		}
		// it must only write to 2 namespaces: LSCC's and the cc that we are deploying/upgrading
		for _, ns := range txRWSet.NsRwSets {
			if ns.NameSpace != "lscc" && ns.NameSpace != cdRWSet.Name && len(ns.KvRwSet.Writes) > 0 {
				return policyErr(fmt.Errorf("LSCC invocation is attempting to write to namespace %s", ns.NameSpace))
			}
		}

		logger.Debugf("Validating %s for cc %s version %s", lsccFunc, cdRWSet.Name, cdRWSet.Version)

		switch lsccFunc {
		case lscc.DEPLOY:

			/******************************************************************/
			/* security check 1 - cc not in the LCCC table of instantiated cc */
			/******************************************************************/
			if ccExistsOnLedger {
				return policyErr(fmt.Errorf("Chaincode %s is already instantiated", cdsArgs.ChaincodeSpec.ChaincodeId.Name))
			}

			/****************************************************************************/
			/* security check 2 - validation of rwset (and of collections if enabled) */
			/****************************************************************************/
			if ac.PrivateChannelData() {
				// do extra validation for collections
				err := vscc.validateRWSetAndCollection(lsccrwset, cdRWSet, lsccArgs, lsccFunc, ac, chid)
				if err != nil {
					return err
				}
			} else {
				// there can only be a single ledger write
				if len(lsccrwset.Writes) != 1 {
					return policyErr(fmt.Errorf("LSCC can only issue a single putState upon deploy"))
				}
			}

			/*****************************************************/
			/* security check 3 - check the instantiation policy */
			/*****************************************************/
			pol := cdRWSet.InstantiationPolicy
			if pol == nil {
				return policyErr(fmt.Errorf("no instantiation policy was specified"))
			}
			// FIXME: could we actually pull the cds package from the
			// file system to verify whether the policy that is specified
			// here is the same as the one on disk?
			// PROS: we prevent attacks where the policy is replaced
			// CONS: this would be a point of non-determinism
			err := vscc.checkInstantiationPolicy(chid, env, pol, payl)
			if err != nil {
				return err
			}

		case lscc.UPGRADE:
			/**************************************************************/
			/* security check 1 - cc in the LCCC table of instantiated cc */
			/**************************************************************/
			if !ccExistsOnLedger {
				return policyErr(fmt.Errorf("Upgrading non-existent chaincode %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name))
			}

			/**********************************************************/
			/* security check 2 - existing cc's version was different */
			/**********************************************************/
			if cdLedger.Version == cdsArgs.ChaincodeSpec.ChaincodeId.Version {
				return policyErr(fmt.Errorf("Existing version of the cc on the ledger (%s) should be different from the upgraded one", cdsArgs.ChaincodeSpec.ChaincodeId.Version))
			}

			/****************************************************************************/
			/* security check 3 validation of rwset (and of collections if enabled) */
			/****************************************************************************/
			// Only in v1.2, a collection can be updated during a chaincode upgrade
			if ac.V1_2Validation() {
				// do extra validation for collections
				err := vscc.validateRWSetAndCollection(lsccrwset, cdRWSet, lsccArgs, lsccFunc, ac, chid)
				if err != nil {
					return err
				}
			} else {
				// there can only be a single ledger write
				if len(lsccrwset.Writes) != 1 {
					return policyErr(fmt.Errorf("LSCC can only issue a single putState upon upgrade"))
				}
			}

			/*****************************************************/
			/* security check 4 - check the instantiation policy */
			/*****************************************************/
			pol := cdLedger.InstantiationPolicy
			if pol == nil {
				return policyErr(fmt.Errorf("No instantiation policy was specified"))
			}
			// FIXME: could we actually pull the cds package from the
			// file system to verify whether the policy that is specified
			// here is the same as the one on disk?
			// PROS: we prevent attacks where the policy is replaced
			// CONS: this would be a point of non-determinism
			err := vscc.checkInstantiationPolicy(chid, env, pol, payl)
			if err != nil {
				return err
			}

			/******************************************************************/
			/* security check 5 - check the instantiation policy in the rwset */
			/******************************************************************/
			if ac.V1_1Validation() {
				polNew := cdRWSet.InstantiationPolicy
				if polNew == nil {
					return policyErr(fmt.Errorf("No instantiation policy was specified"))
				}

				// no point in checking it again if they are the same policy
				if !bytes.Equal(polNew, pol) {
					err = vscc.checkInstantiationPolicy(chid, env, polNew, payl)
					if err != nil {
						return err
					}
				}
			}
		}

		// all is good!
		return nil
	default:
		return policyErr(fmt.Errorf("VSCC error: committing an invocation of function %s of lscc is invalid", lsccFunc))
	}
}

func (vscc *Validator) getInstantiatedCC(chid, ccid string) (cd *ccprovider.ChaincodeData, exists bool, err error) {
	qe, err := vscc.stateFetcher.FetchState()
	if err != nil {
		err = fmt.Errorf("could not retrieve QueryExecutor for channel %s, error %s", chid, err)
		return
	}
	defer qe.Done()
	channelState := &state{qe}
	bytes, err := channelState.GetState("lscc", ccid)
	if err != nil {
		err = fmt.Errorf("could not retrieve state for chaincode %s on channel %s, error %s", ccid, chid, err)
		return
	}

	if bytes == nil {
		return
	}

	cd = &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(bytes, cd)
	if err != nil {
		err = fmt.Errorf("unmarshalling ChaincodeQueryResponse failed, error %s", err)
		return
	}

	exists = true
	return
}

func (vscc *Validator) deduplicateIdentity(cap *pb.ChaincodeActionPayload) ([]*protoutil.SignedData, error) {
	// this is the first part of the signed message
	prespBytes := cap.Action.ProposalResponsePayload

	// build the signature set for the evaluation
	signatureSet := []*protoutil.SignedData{}
	signatureMap := make(map[string]struct{})
	// loop through each of the endorsements and build the signature set
	for _, endorsement := range cap.Action.Endorsements {
		// unmarshal endorser bytes
		serializedIdentity := &msp.SerializedIdentity{}
		if err := proto.Unmarshal(endorsement.Endorser, serializedIdentity); err != nil {
			logger.Errorf("Unmarshal endorser error: %s", err)
			return nil, policyErr(fmt.Errorf("Unmarshal endorser error: %s", err))
		}
		identity := serializedIdentity.Mspid + string(serializedIdentity.IdBytes)
		if _, ok := signatureMap[identity]; ok {
			// Endorsement with the same identity has already been added
			logger.Warningf("Ignoring duplicated identity, Mspid: %s, pem:\n%s", serializedIdentity.Mspid, serializedIdentity.IdBytes)
			continue
		}
		data := make([]byte, len(prespBytes)+len(endorsement.Endorser))
		copy(data, prespBytes)
		copy(data[len(prespBytes):], endorsement.Endorser)
		signatureSet = append(signatureSet, &protoutil.SignedData{
			// set the data that is signed; concatenation of proposal response bytes and endorser ID
			Data: data,
			// set the identity that signs the message: it's the endorser
			Identity: endorsement.Endorser,
			// set the signature
			Signature: endorsement.Signature,
		})
		signatureMap[identity] = struct{}{}
	}

	logger.Debugf("Signature set is of size %d out of %d endorsement(s)", len(signatureSet), len(cap.Action.Endorsements))
	return signatureSet, nil
}

type state struct {
	vs.State
}

// GetState retrieves the value for the given key in the given namespace.
func (s *state) GetState(namespace string, key string) ([]byte, error) {
	values, err := s.GetStateMultipleKeys(namespace, []string{key})
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values[0], nil
}

func policyErr(err error) *commonerrors.VSCCEndorsementPolicyError {
	return &commonerrors.VSCCEndorsementPolicyError{
		Err: err,
	}
}
