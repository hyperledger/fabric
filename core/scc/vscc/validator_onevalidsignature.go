/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vscc

import (
	"fmt"

	"errors"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/scc/lscc"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

var logger = flogging.MustGetLogger("vscc")

const (
	DUPLICATED_IDENTITY_ERROR = "Endorsement policy evaluation failure might be caused by duplicated identities"
)

// ValidatorOneValidSignature implements the default transaction validation policy,
// which is to check the correctness of the read-write set and the endorsement
// signatures
type ValidatorOneValidSignature struct {
	// sccprovider is the interface with which we call
	// methods of the system chaincode package without
	// import cycles
	sccprovider sysccprovider.SystemChaincodeProvider
}

// Init is called once when the chaincode started the first time
func (vscc *ValidatorOneValidSignature) Init(stub shim.ChaincodeStubInterface) pb.Response {
	vscc.sccprovider = sysccprovider.GetSystemChaincodeProvider()

	return shim.Success(nil)
}

// Invoke is called to validate the specified block of transactions
// This validation system chaincode will check the read-write set validity and at least 1
// correct endorsement. Later we can create more validation system
// chaincodes to provide more sophisticated policy processing such as enabling
// policy specification to be coded as a transaction of the chaincode and the client
// selecting which policy to use for validation using parameter function
// @return serialized Block of valid and invalid transactions identified
// Note that Peer calls this function with 3 arguments, where args[0] is the
// function name, args[1] is the Envelope and args[2] is the validation policy
func (vscc *ValidatorOneValidSignature) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	// TODO: document the argument in some white paper or design document
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	// args[2] - serialized policy
	args := stub.GetArgs()
	if len(args) < 3 {
		return shim.Error("Incorrect number of arguments")
	}

	if args[1] == nil {
		return shim.Error("No block to validate")
	}

	if args[2] == nil {
		return shim.Error("No policy supplied")
	}

	logger.Debugf("VSCC invoked")

	// get the envelope...
	env, err := utils.GetEnvelopeFromBlock(args[1])
	if err != nil {
		logger.Errorf("VSCC error: GetEnvelope failed, err %s", err)
		return shim.Error(err.Error())
	}

	// ...and the payload...
	payl, err := utils.GetPayload(env)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return shim.Error(err.Error())
	}

	chdr, err := utils.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return shim.Error(err.Error())
	}

	// get the policy
	mgr := mspmgmt.GetManagerForChain(chdr.ChannelId)
	pProvider := cauthdsl.NewPolicyProvider(mgr)
	policy, _, err := pProvider.NewPolicy(args[2])
	if err != nil {
		logger.Errorf("VSCC error: pProvider.NewPolicy failed, err %s", err)
		return shim.Error(err.Error())
	}

	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type)
		return shim.Error(fmt.Sprintf("Only Endorser Transactions are supported, provided type %d", chdr.Type))
	}

	// ...and the transaction...
	tx, err := utils.GetTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return shim.Error(err.Error())
	}

	// loop through each of the actions within
	for _, act := range tx.Actions {
		cap, err := utils.GetChaincodeActionPayload(act.Payload)
		if err != nil {
			logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
			return shim.Error(err.Error())
		}

		signatureSet, err := vscc.deduplicateIdentity(cap)
		if err != nil {
			return shim.Error(err.Error())
		}

		// evaluate the signature set against the policy
		err = policy.Evaluate(signatureSet)
		if err != nil {
			logger.Warningf("Endorsement policy failure for transaction txid=%s, err: %s", chdr.GetTxId(), err.Error())
			if len(signatureSet) < len(cap.Action.Endorsements) {
				// Warning: duplicated identities exist, endorsement failure might be cause by this reason
				return shim.Error(DUPLICATED_IDENTITY_ERROR)
			}
			return shim.Error(fmt.Sprintf("VSCC error: policy evaluation failed, err %s", err))
		}

		hdrExt, err := utils.GetChaincodeHeaderExtension(payl.Header)
		if err != nil {
			logger.Errorf("VSCC error: GetChaincodeHeaderExtension failed, err %s", err)
			return shim.Error(err.Error())
		}

		// do some extra validation that is specific to lscc
		if hdrExt.ChaincodeId.Name == "lscc" {
			logger.Debugf("VSCC info: doing special validation for LSCC")

			err = vscc.ValidateLSCCInvocation(stub, chdr.ChannelId, env, cap, payl)
			if err != nil {
				logger.Errorf("VSCC error: ValidateLSCCInvocation failed, err %s", err)
				return shim.Error(err.Error())
			}
		}
	}

	logger.Debugf("VSCC exists successfully")

	return shim.Success(nil)
}

// checkInstantiationPolicy evaluates an instantiation policy against a signed proposal
func (vscc *ValidatorOneValidSignature) checkInstantiationPolicy(chainName string, env *common.Envelope, instantiationPolicy []byte, payl *common.Payload) error {
	// create a policy object from the policy bytes
	mgr := mspmgmt.GetManagerForChain(chainName)
	if mgr == nil {
		return fmt.Errorf("MSP manager for channel %s is nil, aborting", chainName)
	}

	npp := cauthdsl.NewPolicyProvider(mgr)
	instPol, _, err := npp.NewPolicy(instantiationPolicy)
	if err != nil {
		return err
	}

	logger.Debugf("VSCC info: checkInstantiationPolicy starts, policy is %#v", instPol)

	// get the signature header
	shdr, err := utils.GetSignatureHeader(payl.Header.SignatureHeader)
	if err != nil {
		return err
	}

	// construct signed data we can evaluate the instantiation policy against
	sd := []*common.SignedData{&common.SignedData{
		Data:      env.Payload,
		Identity:  shdr.Creator,
		Signature: env.Signature,
	}}
	err = instPol.Evaluate(sd)
	if err != nil {
		return fmt.Errorf("chaincode instantiation policy violated, error %s", err)
	}
	return nil
}

func (vscc *ValidatorOneValidSignature) ValidateLSCCInvocation(stub shim.ChaincodeStubInterface, chid string, env *common.Envelope, cap *pb.ChaincodeActionPayload, payl *common.Payload) error {
	cpp, err := utils.GetChaincodeProposalPayload(cap.ChaincodeProposalPayload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeProposalPayload failed, err %s", err)
		return err
	}

	cis := &pb.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(cpp.Input, cis)
	if err != nil {
		logger.Errorf("VSCC error: Unmarshal ChaincodeInvocationSpec failed, err %s", err)
		return err
	}

	if cis.ChaincodeSpec == nil ||
		cis.ChaincodeSpec.Input == nil ||
		cis.ChaincodeSpec.Input.Args == nil {
		logger.Errorf("VSCC error: committing invalid vscc invocation")
		return fmt.Errorf("VSCC error: committing invalid vscc invocation")
	}

	lsccFunc := string(cis.ChaincodeSpec.Input.Args[0])
	lsccArgs := cis.ChaincodeSpec.Input.Args[1:]

	logger.Debugf("VSCC info: ValidateLSCCInvocation acting on %s %#v", lsccFunc, lsccArgs)

	switch lsccFunc {
	case lscc.UPGRADE, lscc.DEPLOY:
		logger.Debugf("VSCC info: validating invocation of lscc function %s on arguments %#v", lsccFunc, lsccArgs)

		if len(lsccArgs) < 2 || len(lsccArgs) > 5 {
			return fmt.Errorf("Wrong number of arguments for invocation lscc(%s): expected between 2 and 5, received %d", lsccFunc, len(lsccArgs))
		}

		cdsArgs, err := utils.GetChaincodeDeploymentSpec(lsccArgs[1])
		if err != nil {
			return fmt.Errorf("GetChaincodeDeploymentSpec error %s", err)
		}

		if cdsArgs == nil || cdsArgs.ChaincodeSpec == nil || cdsArgs.ChaincodeSpec.ChaincodeId == nil ||
			cap.Action == nil || cap.Action.ProposalResponsePayload == nil {
			return fmt.Errorf("VSCC error: invocation of lscc(%s) does not have appropriate arguments", lsccFunc)
		}

		// get the rwset
		pRespPayload, err := utils.GetProposalResponsePayload(cap.Action.ProposalResponsePayload)
		if err != nil {
			return fmt.Errorf("GetProposalResponsePayload error %s", err)
		}
		if pRespPayload.Extension == nil {
			return fmt.Errorf("nil pRespPayload.Extension")
		}
		respPayload, err := utils.GetChaincodeAction(pRespPayload.Extension)
		if err != nil {
			return fmt.Errorf("GetChaincodeAction error %s", err)
		}
		txRWSet := &rwsetutil.TxRwSet{}
		if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
			return fmt.Errorf("txRWSet.FromProtoBytes error %s", err)
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
			return err
		}

		/******************************************/
		/* security check 0 - validation of rwset */
		/******************************************/
		// there has to be one
		if lsccrwset == nil {
			return errors.New("No read write set for lscc was found")
		}
		// there can only be a single one
		if len(lsccrwset.Writes) != 1 {
			return errors.New("LSCC can only issue a single putState upon deploy/upgrade")
		}
		// the key name must be the chaincode id
		if lsccrwset.Writes[0].Key != cdsArgs.ChaincodeSpec.ChaincodeId.Name {
			return fmt.Errorf("Expected key %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name, lsccrwset.Writes[0].Key)
		}
		// the value must be a ChaincodeData struct
		cdRWSet := &ccprovider.ChaincodeData{}
		err = proto.Unmarshal(lsccrwset.Writes[0].Value, cdRWSet)
		if err != nil {
			return fmt.Errorf("Unmarhsalling of ChaincodeData failed, error %s", err)
		}
		// the name must match
		if cdRWSet.Name != cdsArgs.ChaincodeSpec.ChaincodeId.Name {
			return fmt.Errorf("Expected cc name %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name, cdRWSet.Name)
		}
		// the version must match
		if cdRWSet.Version != cdsArgs.ChaincodeSpec.ChaincodeId.Version {
			return fmt.Errorf("Expected cc version %s, found %s", cdsArgs.ChaincodeSpec.ChaincodeId.Version, cdRWSet.Version)
		}
		// it must only write to 2 namespaces: LSCC's and the cc that we are deploying/upgrading
		for _, ns := range txRWSet.NsRwSets {
			if ns.NameSpace != "lscc" && ns.NameSpace != cdRWSet.Name && len(ns.KvRwSet.Writes) > 0 {
				return fmt.Errorf("LSCC invocation is attempting to write to namespace %s", ns.NameSpace)
			}
		}

		logger.Debugf("Validating %s for cc %s version %s", lsccFunc, cdRWSet.Name, cdRWSet.Version)

		switch lsccFunc {
		case lscc.DEPLOY:
			/*****************************************************/
			/* security check 1 - check the instantiation policy */
			/*****************************************************/
			pol := cdRWSet.InstantiationPolicy
			if pol == nil {
				return fmt.Errorf("No installation policy was specified")
			}
			// FIXME: could we actually pull the cds package from the
			// file system to verify whether the policy that is specified
			// here is the same as the one on disk?
			// PROS: we prevent attacks where the policy is replaced
			// CONS: this would be a point of non-determinism
			err = vscc.checkInstantiationPolicy(chid, env, pol, payl)
			if err != nil {
				return err
			}

			/******************************************************************/
			/* security check 2 - cc not in the LCCC table of instantiated cc */
			/******************************************************************/
			if ccExistsOnLedger {
				return fmt.Errorf("Chaincode %s is already instantiated", cdsArgs.ChaincodeSpec.ChaincodeId.Name)
			}

		case lscc.UPGRADE:
			/**************************************************************/
			/* security check 1 - cc in the LCCC table of instantiated cc */
			/**************************************************************/
			if !ccExistsOnLedger {
				return fmt.Errorf("Upgrading non-existent chaincode %s", cdsArgs.ChaincodeSpec.ChaincodeId.Name)
			}

			/*****************************************************/
			/* security check 2 - check the instantiation policy */
			/*****************************************************/
			pol := cdLedger.InstantiationPolicy
			if pol == nil {
				return fmt.Errorf("No installation policy was specified")
			}
			// FIXME: could we actually pull the cds package from the
			// file system to verify whether the policy that is specified
			// here is the same as the one on disk?
			// PROS: we prevent attacks where the policy is replaced
			// CONS: this would be a point of non-determinism
			err = vscc.checkInstantiationPolicy(chid, env, pol, payl)
			if err != nil {
				return err
			}

			/**********************************************************/
			/* security check 3 - existing cc's version was different */
			/**********************************************************/
			if cdLedger.Version == cdsArgs.ChaincodeSpec.ChaincodeId.Version {
				return fmt.Errorf("Existing version of the cc on the ledger (%s) should be different from the upgraded one", cdsArgs.ChaincodeSpec.ChaincodeId.Version)
			}
		}

		// all is good!
		return nil
	default:
		return fmt.Errorf("VSCC error: committing an invocation of function %s of lscc is invalid", lsccFunc)
	}
}

func (vscc *ValidatorOneValidSignature) getInstantiatedCC(chid, ccid string) (cd *ccprovider.ChaincodeData, exists bool, err error) {
	qe, err := vscc.sccprovider.GetQueryExecutorForLedger(chid)
	if err != nil {
		err = fmt.Errorf("Could not retrieve QueryExecutor for channel %s, error %s", chid, err)
		return
	}
	defer qe.Done()

	bytes, err := qe.GetState("lscc", ccid)
	if err != nil {
		err = fmt.Errorf("Could not retrieve state for chaincode %s on channel %s, error %s", ccid, chid, err)
		return
	}

	if bytes == nil {
		return
	}

	cd = &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(bytes, cd)
	if err != nil {
		err = fmt.Errorf("Unmarshalling ChaincodeQueryResponse failed, error %s", err)
		return
	}

	exists = true
	return
}

func (vscc *ValidatorOneValidSignature) deduplicateIdentity(cap *pb.ChaincodeActionPayload) ([]*common.SignedData, error) {
	// this is the first part of the signed message
	prespBytes := cap.Action.ProposalResponsePayload

	// build the signature set for the evaluation
	signatureSet := []*common.SignedData{}
	signatureMap := make(map[string]struct{})
	// loop through each of the endorsements and build the signature set
	for _, endorsement := range cap.Action.Endorsements {
		//unmarshal endorser bytes
		serializedIdentity := &msp.SerializedIdentity{}
		if err := proto.Unmarshal(endorsement.Endorser, serializedIdentity); err != nil {
			logger.Errorf("Unmarshal endorser error: %s", err)
			return nil, fmt.Errorf("Unmarshal endorser error: %s", err)
		}
		identity := serializedIdentity.Mspid + string(serializedIdentity.IdBytes)
		if _, ok := signatureMap[identity]; ok {
			// Endorsement with the same identity has already been added
			logger.Warningf("Ignoring duplicated identity, Mspid: %s, pem:\n%s", serializedIdentity.Mspid, serializedIdentity.IdBytes)
			continue
		}
		signatureSet = append(signatureSet, &common.SignedData{
			// set the data that is signed; concatenation of proposal response bytes and endorser ID
			Data: append(prespBytes, endorsement.Endorser...),
			// set the identity that signs the message: it's the endorser
			Identity: endorsement.Endorser,
			// set the signature
			Signature: endorsement.Signature})
		signatureMap[identity] = struct{}{}
	}

	logger.Debugf("Signature set is of size %d out of %d endorsement(s)", len(signatureSet), len(cap.Action.Endorsements))
	return signatureSet, nil
}
