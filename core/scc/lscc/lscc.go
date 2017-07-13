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

package lscc

import (
	"fmt"
	"regexp"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/policyprovider"
	"github.com/hyperledger/fabric/msp/mgmt"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

//The life cycle system chaincode manages chaincodes deployed
//on this peer. It manages chaincodes via Invoke proposals.
//     "Args":["deploy",<ChaincodeDeploymentSpec>]
//     "Args":["upgrade",<ChaincodeDeploymentSpec>]
//     "Args":["stop",<ChaincodeInvocationSpec>]
//     "Args":["start",<ChaincodeInvocationSpec>]

var logger = flogging.MustGetLogger("lscc")

const (
	//CHAINCODETABLE prefix for chaincode tables
	CHAINCODETABLE = "chaincodes"

	//chaincode lifecyle commands

	//INSTALL install command
	INSTALL = "install"

	//DEPLOY deploy command
	DEPLOY = "deploy"

	//UPGRADE upgrade chaincode
	UPGRADE = "upgrade"

	//GETCCINFO get chaincode
	GETCCINFO = "getid"

	//GETDEPSPEC get ChaincodeDeploymentSpec
	GETDEPSPEC = "getdepspec"

	//GETCCDATA get ChaincodeData
	GETCCDATA = "getccdata"

	//GETCHAINCODES gets the instantiated chaincodes on a channel
	GETCHAINCODES = "getchaincodes"

	//GETINSTALLEDCHAINCODES gets the installed chaincodes on a peer
	GETINSTALLEDCHAINCODES = "getinstalledchaincodes"

	allowedCharsChaincodeName = "[A-Za-z0-9_-]+"
	allowedCharsVersion       = "[A-Za-z0-9_.-]+"
)

//---------- the LSCC -----------------

// LifeCycleSysCC implements chaincode lifecycle and policies around it
type LifeCycleSysCC struct {
	// sccprovider is the interface with which we call
	// methods of the system chaincode package without
	// import cycles
	sccprovider sysccprovider.SystemChaincodeProvider

	// policyChecker is the interface used to perform
	// access control
	policyChecker policy.PolicyChecker
}

//----------------errors---------------

//AlreadyRegisteredErr Already registered error
type AlreadyRegisteredErr string

func (f AlreadyRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(f))
}

//InvalidFunctionErr invalid function error
type InvalidFunctionErr string

func (f InvalidFunctionErr) Error() string {
	return fmt.Sprintf("invalid function to lscc %s", string(f))
}

//InvalidArgsLenErr invalid arguments length error
type InvalidArgsLenErr int

func (i InvalidArgsLenErr) Error() string {
	return fmt.Sprintf("invalid number of argument to lscc %d", int(i))
}

//InvalidArgsErr invalid arguments error
type InvalidArgsErr int

func (i InvalidArgsErr) Error() string {
	return fmt.Sprintf("invalid argument (%d) to lscc", int(i))
}

//TXExistsErr transaction exists error
type TXExistsErr string

func (t TXExistsErr) Error() string {
	return fmt.Sprintf("transaction exists %s", string(t))
}

//TXNotFoundErr transaction not found error
type TXNotFoundErr string

func (t TXNotFoundErr) Error() string {
	return fmt.Sprintf("transaction not found %s", string(t))
}

//InvalidDeploymentSpecErr invalide chaincode deployment spec error
type InvalidDeploymentSpecErr string

func (f InvalidDeploymentSpecErr) Error() string {
	return fmt.Sprintf("invalid deployment spec : %s", string(f))
}

//ExistsErr chaincode exists error
type ExistsErr string

func (t ExistsErr) Error() string {
	return fmt.Sprintf("chaincode exists %s", string(t))
}

//NotFoundErr chaincode not registered with LSCC error
type NotFoundErr string

func (t NotFoundErr) Error() string {
	return fmt.Sprintf("could not find chaincode with name '%s'", string(t))
}

//InvalidChainNameErr invalid chain name error
type InvalidChainNameErr string

func (f InvalidChainNameErr) Error() string {
	return fmt.Sprintf("invalid chain name %s", string(f))
}

//InvalidChaincodeNameErr invalid chaincode name error
type InvalidChaincodeNameErr string

func (f InvalidChaincodeNameErr) Error() string {
	return fmt.Sprintf("invalid chaincode name '%s'. Names can only consist of alphanumerics, '_', and '-'", string(f))
}

//EmptyChaincodeNameErr trying to upgrade to same version of Chaincode
type EmptyChaincodeNameErr string

func (f EmptyChaincodeNameErr) Error() string {
	return fmt.Sprint("chaincode name not provided")
}

//InvalidVersionErr invalid version error
type InvalidVersionErr string

func (f InvalidVersionErr) Error() string {
	return fmt.Sprintf("invalid chaincode version '%s'. Versions can only consist of alphanumerics, '_',  '-', and '.'", string(f))
}

//ChaincodeMismatchErr chaincode name from two places don't match
type ChaincodeMismatchErr string

func (f ChaincodeMismatchErr) Error() string {
	return fmt.Sprintf("chaincode name mismatch %s", string(f))
}

//EmptyVersionErr empty version error
type EmptyVersionErr string

func (f EmptyVersionErr) Error() string {
	return fmt.Sprintf("version not provided for chaincode with name '%s'", string(f))
}

//MarshallErr error marshaling/unmarshalling
type MarshallErr string

func (m MarshallErr) Error() string {
	return fmt.Sprintf("error while marshalling %s", string(m))
}

//IdenticalVersionErr trying to upgrade to same version of Chaincode
type IdenticalVersionErr string

func (f IdenticalVersionErr) Error() string {
	return fmt.Sprintf("version already exists for chaincode with name '%s'", string(f))
}

//InvalidCCOnFSError error due to mismatch between fingerprint on lscc and installed CC
type InvalidCCOnFSError string

func (f InvalidCCOnFSError) Error() string {
	return fmt.Sprintf("chaincode fingerprint mismatch %s", string(f))
}

//InstantiationPolicyViolatedErr when chaincode instantiation policy has been violated on instantiate or upgrade
type InstantiationPolicyViolatedErr string

func (f InstantiationPolicyViolatedErr) Error() string {
	return fmt.Sprintf("chaincode instantiation policy violated(%s)", string(f))
}

//InstantiationPolicyMissing when no existing instantiation policy is found when upgrading CC
type InstantiationPolicyMissing string

func (f InstantiationPolicyMissing) Error() string {
	return "instantiation policy missing"
}

//-------------- helper functions ------------------
//create the chaincode on the given chain
func (lscc *LifeCycleSysCC) createChaincode(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
	return lscc.putChaincodeData(stub, cd)
}

//upgrade the chaincode on the given chain
func (lscc *LifeCycleSysCC) upgradeChaincode(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
	return lscc.putChaincodeData(stub, cd)
}

//create the chaincode on the given chain
func (lscc *LifeCycleSysCC) putChaincodeData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
	// check that escc and vscc are real system chaincodes
	if !lscc.sccprovider.IsSysCC(string(cd.Escc)) {
		return fmt.Errorf("%s is not a valid endorsement system chaincode", string(cd.Escc))
	}
	if !lscc.sccprovider.IsSysCC(string(cd.Vscc)) {
		return fmt.Errorf("%s is not a valid validation system chaincode", string(cd.Vscc))
	}

	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		return err
	}

	if cdbytes == nil {
		return MarshallErr(cd.Name)
	}

	err = stub.PutState(cd.Name, cdbytes)

	return err
}

//checks for existence of chaincode on the given channel
func (lscc *LifeCycleSysCC) getCCInstance(stub shim.ChaincodeStubInterface, ccname string) ([]byte, error) {
	cdbytes, err := stub.GetState(ccname)
	if err != nil {
		return nil, TXNotFoundErr(err.Error())
	}
	if cdbytes == nil {
		return nil, NotFoundErr(ccname)
	}

	return cdbytes, nil
}

//gets the cd out of the bytes
func (lscc *LifeCycleSysCC) getChaincodeData(ccname string, cdbytes []byte) (*ccprovider.ChaincodeData, error) {
	cd := &ccprovider.ChaincodeData{}
	err := proto.Unmarshal(cdbytes, cd)
	if err != nil {
		return nil, MarshallErr(ccname)
	}

	//this should not happen but still a sanity check is not a bad thing
	if cd.Name != ccname {
		return nil, ChaincodeMismatchErr(fmt.Sprintf("%s!=%s", ccname, cd.Name))
	}

	return cd, nil
}

//checks for existence of chaincode on the given chain
func (lscc *LifeCycleSysCC) getCCCode(ccname string, cdbytes []byte) (*ccprovider.ChaincodeData, *pb.ChaincodeDeploymentSpec, []byte, error) {
	cd, err := lscc.getChaincodeData(ccname, cdbytes)
	if err != nil {
		return nil, nil, nil, err
	}

	ccpack, err := ccprovider.GetChaincodeFromFS(ccname, cd.Version)
	if err != nil {
		return nil, nil, nil, InvalidDeploymentSpecErr(err.Error())
	}

	//this is the big test and the reason every launch should go through
	//getChaincode call. We validate the chaincode entry against the
	//the chaincode in FS
	if err = ccpack.ValidateCC(cd); err != nil {
		return nil, nil, nil, InvalidCCOnFSError(err.Error())
	}

	//these are guaranteed to be non-nil because we got a valid ccpack
	depspec := ccpack.GetDepSpec()
	depspecbytes := ccpack.GetDepSpecBytes()

	return cd, depspec, depspecbytes, nil
}

// getChaincodes returns all chaincodes instantiated on this LSCC's channel
func (lscc *LifeCycleSysCC) getChaincodes(stub shim.ChaincodeStubInterface) pb.Response {
	// get all rows from LSCC
	itr, err := stub.GetStateByRange("", "")

	if err != nil {
		return shim.Error(err.Error())
	}
	defer itr.Close()

	// array to store metadata for all chaincode entries from LSCC
	var ccInfoArray []*pb.ChaincodeInfo

	for itr.HasNext() {
		response, err := itr.Next()
		if err != nil {
			return shim.Error(err.Error())
		}

		ccdata := &ccprovider.ChaincodeData{}
		if err = proto.Unmarshal(response.Value, ccdata); err != nil {
			return shim.Error(err.Error())
		}

		var path string
		var input string

		//if chaincode is not installed on the system we won't have
		//data beyond name and version
		ccpack, err := ccprovider.GetChaincodeFromFS(ccdata.Name, ccdata.Version)
		if err == nil {
			path = ccpack.GetDepSpec().GetChaincodeSpec().ChaincodeId.Path
			input = ccpack.GetDepSpec().GetChaincodeSpec().Input.String()
		}

		ccInfo := &pb.ChaincodeInfo{Name: ccdata.Name, Version: ccdata.Version, Path: path, Input: input, Escc: ccdata.Escc, Vscc: ccdata.Vscc}

		// add this specific chaincode's metadata to the array of all chaincodes
		ccInfoArray = append(ccInfoArray, ccInfo)
	}
	// add array with info about all instantiated chaincodes to the query
	// response proto
	cqr := &pb.ChaincodeQueryResponse{Chaincodes: ccInfoArray}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}

// getInstalledChaincodes returns all chaincodes installed on the peer
func (lscc *LifeCycleSysCC) getInstalledChaincodes() pb.Response {
	// get chaincode query response proto which contains information about all
	// installed chaincodes
	cqr, err := ccprovider.GetInstalledChaincodes()
	if err != nil {
		return shim.Error(err.Error())
	}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}

//do access control
func (lscc *LifeCycleSysCC) acl(stub shim.ChaincodeStubInterface, chainname string, cds *pb.ChaincodeDeploymentSpec) error {
	return nil
}

//check validity of chain name
func (lscc *LifeCycleSysCC) isValidChainName(chainname string) bool {
	//TODO we probably need more checks
	if chainname == "" {
		return false
	}
	return true
}

// isValidChaincodeName checks the validity of chaincode name. Chaincode names
// should never be blank and should only consist of alphanumerics, '_', and '-'
func (lscc *LifeCycleSysCC) isValidChaincodeName(chaincodeName string) error {
	if chaincodeName == "" {
		return EmptyChaincodeNameErr("")
	}

	if !isValidCCNameOrVersion(chaincodeName, allowedCharsChaincodeName) {
		return InvalidChaincodeNameErr(chaincodeName)
	}

	return nil
}

// isValidChaincodeVersion checks the validity of chaincode version. Versions
// should never be blank and should only consist of alphanumerics, '_',  '-',
// and '.'
func (lscc *LifeCycleSysCC) isValidChaincodeVersion(chaincodeName string, version string) error {
	if version == "" {
		return EmptyVersionErr(chaincodeName)
	}

	if !isValidCCNameOrVersion(version, allowedCharsVersion) {
		return InvalidVersionErr(version)
	}

	return nil
}

func isValidCCNameOrVersion(ccNameOrVersion string, regExp string) bool {
	re, _ := regexp.Compile(regExp)

	matched := re.FindString(ccNameOrVersion)
	if len(matched) != len(ccNameOrVersion) {
		return false
	}

	return true
}

// executeInstall implements the "install" Invoke transaction
func (lscc *LifeCycleSysCC) executeInstall(stub shim.ChaincodeStubInterface, ccbytes []byte) error {
	ccpack, err := ccprovider.GetCCPackage(ccbytes)
	if err != nil {
		return err
	}

	cds := ccpack.GetDepSpec()

	if cds == nil {
		return fmt.Errorf("nil deployment spec from from the CC package")
	}

	if err = lscc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeId.Name); err != nil {
		return err
	}

	if err = lscc.isValidChaincodeVersion(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version); err != nil {
		return err
	}

	//everything checks out..lets write the package to the FS
	if err = ccpack.PutChaincodeToFS(); err != nil {
		return fmt.Errorf("Error installing chaincode code %s:%s(%s)", cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version, err)
	}

	return err
}

// getInstantiationPolicy retrieves the instantiation policy from a SignedCDSPackage
func (lscc *LifeCycleSysCC) getInstantiationPolicy(channel string, ccpack ccprovider.CCPackage) ([]byte, error) {
	var ip []byte
	var err error
	// if ccpack is a SignedCDSPackage, return its IP, otherwise use a default IP
	sccpack, isSccpack := ccpack.(*ccprovider.SignedCDSPackage)
	if isSccpack {
		ip = sccpack.GetInstantiationPolicy()
		if ip == nil {
			return nil, fmt.Errorf("Instantiation policy cannot be null for a SignedCCDeploymentSpec")
		}
	} else {
		// the default instantiation policy allows any of the channel MSP admins
		// to be able to instantiate
		mspids := peer.GetMSPIDs(channel)

		p := cauthdsl.SignedByAnyAdmin(mspids)
		ip, err = utils.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("Error marshalling default instantiation policy")
		}

	}
	return ip, nil
}

// checkInstantiationPolicy evaluates an instantiation policy against a signed proposal
func (lscc *LifeCycleSysCC) checkInstantiationPolicy(stub shim.ChaincodeStubInterface, chainName string, instantiationPolicy []byte) error {
	// create a policy object from the policy bytes
	mgr := mspmgmt.GetManagerForChain(chainName)
	if mgr == nil {
		return fmt.Errorf("Error checking chaincode instantiation policy: MSP manager for chain %s not found", chainName)
	}
	npp := cauthdsl.NewPolicyProvider(mgr)
	instPol, _, err := npp.NewPolicy(instantiationPolicy)
	if err != nil {
		return err
	}
	// get the signed instantiation proposal
	signedProp, err := stub.GetSignedProposal()
	if err != nil {
		return err
	}
	proposal, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return err
	}
	// get the signature header of the proposal
	header, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return err
	}
	shdr, err := utils.GetSignatureHeader(header.SignatureHeader)
	if err != nil {
		return err
	}
	// construct signed data we can evaluate the instantiation policy against
	sd := []*common.SignedData{&common.SignedData{
		Data:      signedProp.ProposalBytes,
		Identity:  shdr.Creator,
		Signature: signedProp.Signature,
	}}
	err = instPol.Evaluate(sd)
	if err != nil {
		return InstantiationPolicyViolatedErr(err.Error())
	}
	return nil
}

// executeDeploy implements the "instantiate" Invoke transaction
func (lscc *LifeCycleSysCC) executeDeploy(stub shim.ChaincodeStubInterface, chainname string, depSpec []byte, policy []byte, escc []byte, vscc []byte) (*ccprovider.ChaincodeData, error) {
	cds, err := utils.GetChaincodeDeploymentSpec(depSpec)

	if err != nil {
		return nil, err
	}

	if err = lscc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeId.Name); err != nil {
		return nil, err
	}

	if err = lscc.isValidChaincodeVersion(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version); err != nil {
		return nil, err
	}

	if err = lscc.acl(stub, chainname, cds); err != nil {
		return nil, err
	}

	//just test for existence of the chaincode in the LSCC
	_, err = lscc.getCCInstance(stub, cds.ChaincodeSpec.ChaincodeId.Name)
	if err == nil {
		return nil, ExistsErr(cds.ChaincodeSpec.ChaincodeId.Name)
	}

	//get the chaincode from the FS
	ccpack, err := ccprovider.GetChaincodeFromFS(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version)
	if err != nil {
		return nil, fmt.Errorf("cannot get package for the chaincode to be instantiated (%s:%s)-%s", cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version, err)
	}

	//this is guarantees to be not nil
	cd := ccpack.GetChaincodeData()

	//retain chaincode specific data and fill channel specific ones
	cd.Escc = string(escc)
	cd.Vscc = string(vscc)
	cd.Policy = policy

	// retrieve and evaluate instantiation policy
	cd.InstantiationPolicy, err = lscc.getInstantiationPolicy(chainname, ccpack)
	if err != nil {
		return nil, err
	}
	err = lscc.checkInstantiationPolicy(stub, chainname, cd.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	err = lscc.createChaincode(stub, cd)

	return cd, err
}

// executeUpgrade implements the "upgrade" Invoke transaction.
func (lscc *LifeCycleSysCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, depSpec []byte, policy []byte, escc []byte, vscc []byte) (*ccprovider.ChaincodeData, error) {
	cds, err := utils.GetChaincodeDeploymentSpec(depSpec)
	if err != nil {
		return nil, err
	}

	if err = lscc.acl(stub, chainName, cds); err != nil {
		return nil, err
	}

	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name
	if err = lscc.isValidChaincodeName(chaincodeName); err != nil {
		return nil, err
	}

	if err = lscc.isValidChaincodeVersion(chaincodeName, cds.ChaincodeSpec.ChaincodeId.Version); err != nil {
		return nil, err
	}

	// check for existence of chaincode instance only (it has to exist on the channel)
	// we dont care about the old chaincode on the FS. In particular, user may even
	// have deleted it
	cdbytes, _ := lscc.getCCInstance(stub, chaincodeName)
	if cdbytes == nil {
		return nil, NotFoundErr(chainName)
	}

	//we need the cd to compare the version
	cd, err := lscc.getChaincodeData(chaincodeName, cdbytes)
	if err != nil {
		return nil, err
	}

	//do not upgrade if same version
	if cd.Version == cds.ChaincodeSpec.ChaincodeId.Version {
		return nil, IdenticalVersionErr(cds.ChaincodeSpec.ChaincodeId.Name)
	}

	//do not upgrade if instantiation policy is violated
	if cd.InstantiationPolicy == nil {
		return nil, InstantiationPolicyMissing("")
	}
	err = lscc.checkInstantiationPolicy(stub, chainName, cd.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	ccpack, err := ccprovider.GetChaincodeFromFS(chaincodeName, cds.ChaincodeSpec.ChaincodeId.Version)
	if err != nil {
		return nil, fmt.Errorf("cannot get package for the chaincode to be upgraded (%s:%s)-%s", chaincodeName, cds.ChaincodeSpec.ChaincodeId.Version, err)
	}

	//get the new cd to upgrade to this is guaranteed to be not nil
	cd = ccpack.GetChaincodeData()

	//retain chaincode specific data and fill channel specific ones
	cd.Escc = string(escc)
	cd.Vscc = string(vscc)
	cd.Policy = policy

	// retrieve and evaluate new instantiation policy
	cd.InstantiationPolicy, err = lscc.getInstantiationPolicy(chainName, ccpack)
	if err != nil {
		return nil, err
	}
	err = lscc.checkInstantiationPolicy(stub, chainName, cd.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	err = lscc.upgradeChaincode(stub, cd)
	if err != nil {
		return nil, err
	}

	return cd, nil
}

//-------------- the chaincode stub interface implementation ----------

//Init only initializes the system chaincode provider
func (lscc *LifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	lscc.sccprovider = sysccprovider.GetSystemChaincodeProvider()

	// Init policy checker for access control
	lscc.policyChecker = policyprovider.GetPolicyChecker()

	return shim.Success(nil)
}

// Invoke implements lifecycle functions "deploy", "start", "stop", "upgrade".
// Deploy's arguments -  {[]byte("deploy"), []byte(<chainname>), <unmarshalled pb.ChaincodeDeploymentSpec>}
//
// Invoke also implements some query-like functions
// Get chaincode arguments -  {[]byte("getid"), []byte(<chainname>), []byte(<chaincodename>)}
func (lscc *LifeCycleSysCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 1 {
		return shim.Error(InvalidArgsLenErr(len(args)).Error())
	}

	function := string(args[0])

	// Handle ACL:
	// 1. get the signed proposal
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed retrieving signed proposal on executing %s with error %s", function, err))
	}

	switch function {
	case INSTALL:
		if len(args) < 2 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		// 2. check local MSP Admins policy
		if err = lscc.policyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("Authorization for INSTALL has been denied (error-%s)", err))
		}

		depSpec := args[1]

		err := lscc.executeInstall(stub, depSpec)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success([]byte("OK"))
	case DEPLOY:
		if len(args) < 3 || len(args) > 6 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		// TODO: add access control check
		// once the instantiation process will be completed.

		//chain the chaincode shoud be associated with. It
		//should be created with a register call
		chainname := string(args[1])

		if !lscc.isValidChainName(chainname) {
			return shim.Error(InvalidChainNameErr(chainname).Error())
		}

		depSpec := args[2]

		// optional arguments here (they can each be nil and may or may not be present)
		// args[3] is a marshalled SignaturePolicyEnvelope representing the endorsement policy
		// args[4] is the name of escc
		// args[5] is the name of vscc
		var policy []byte
		if len(args) > 3 && len(args[3]) > 0 {
			policy = args[3]
		} else {
			p := cauthdsl.SignedByAnyMember(peer.GetMSPIDs(chainname))
			policy, err = utils.Marshal(p)
			if err != nil {
				return shim.Error(err.Error())
			}
		}

		var escc []byte
		if len(args) > 4 && args[4] != nil {
			escc = args[4]
		} else {
			escc = []byte("escc")
		}

		var vscc []byte
		if len(args) > 5 && args[5] != nil {
			vscc = args[5]
		} else {
			vscc = []byte("vscc")
		}

		cd, err := lscc.executeDeploy(stub, chainname, depSpec, policy, escc, vscc)
		if err != nil {
			return shim.Error(err.Error())
		}
		cdbytes, err := proto.Marshal(cd)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(cdbytes)
	case UPGRADE:
		if len(args) < 3 || len(args) > 6 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		chainname := string(args[1])
		if !lscc.isValidChainName(chainname) {
			return shim.Error(InvalidChainNameErr(chainname).Error())
		}

		// TODO: add access control check
		// once the instantiation process will be completed.

		depSpec := args[2]

		// optional arguments here (they can each be nil and may or may not be present)
		// args[3] is a marshalled SignaturePolicyEnvelope representing the endorsement policy
		// args[4] is the name of escc
		// args[5] is the name of vscc
		var policy []byte
		if len(args) > 3 && len(args[3]) > 0 {
			policy = args[3]
		} else {
			p := cauthdsl.SignedByAnyMember(peer.GetMSPIDs(chainname))
			policy, err = utils.Marshal(p)
			if err != nil {
				return shim.Error(err.Error())
			}
		}

		var escc []byte
		if len(args) > 4 && args[4] != nil {
			escc = args[4]
		} else {
			escc = []byte("escc")
		}

		var vscc []byte
		if len(args) > 5 && args[5] != nil {
			vscc = args[5]
		} else {
			vscc = []byte("vscc")
		}

		cd, err := lscc.executeUpgrade(stub, chainname, depSpec, policy, escc, vscc)
		if err != nil {
			return shim.Error(err.Error())
		}
		cdbytes, err := proto.Marshal(cd)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(cdbytes)
	case GETCCINFO, GETDEPSPEC, GETCCDATA:
		if len(args) != 3 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		chain := string(args[1])
		ccname := string(args[2])

		// 2. check local Channel Readers policy
		// Notice that this information are already available on the ledger
		// therefore we enforce here that the caller is reader of the channel.
		if err = lscc.policyChecker.CheckPolicy(chain, policies.ChannelApplicationReaders, sp); err != nil {
			return shim.Error(fmt.Sprintf("Authorization for %s on channel %s has been denied with error %s", function, args[1], err))
		}

		cdbytes, err := lscc.getCCInstance(stub, ccname)
		if err != nil {
			logger.Errorf("error getting chaincode %s on channel: %s(err:%s)", ccname, chain, err)
			return shim.Error(err.Error())
		}

		switch function {
		case GETCCINFO:
			cd, err := lscc.getChaincodeData(ccname, cdbytes)
			if err != nil {
				return shim.Error(err.Error())
			}
			return shim.Success([]byte(cd.Name))
		case GETCCDATA:
			return shim.Success(cdbytes)
		default:
			_, _, depspecbytes, err := lscc.getCCCode(ccname, cdbytes)
			if err != nil {
				return shim.Error(err.Error())
			}
			return shim.Success(depspecbytes)
		}
	case GETCHAINCODES:
		if len(args) != 1 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		// 2. check local MSP Admins policy
		if err = lscc.policyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("Authorization for GETCHAINCODES on channel %s has been denied with error %s", args[0], err))
		}

		return lscc.getChaincodes(stub)
	case GETINSTALLEDCHAINCODES:
		if len(args) != 1 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		// 2. check local MSP Admins policy
		if err = lscc.policyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("Authorization for GETINSTALLEDCHAINCODES on channel %s has been denied with error %s", args[0], err))
		}

		return lscc.getInstalledChaincodes()
	}

	return shim.Error(InvalidFunctionErr(function).Error())
}
