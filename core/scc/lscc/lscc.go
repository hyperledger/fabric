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
	"path/filepath"
	"regexp"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	ccmetadata "github.com/hyperledger/fabric/core/common/ccprovider/metadata"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/policyprovider"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//The life cycle system chaincode manages chaincodes deployed
//on this peer. It manages chaincodes via Invoke proposals.
//     "Args":["deploy",<ChaincodeDeploymentSpec>]
//     "Args":["upgrade",<ChaincodeDeploymentSpec>]
//     "Args":["stop",<ChaincodeInvocationSpec>]
//     "Args":["start",<ChaincodeInvocationSpec>]

var logger = flogging.MustGetLogger("lscc")

const (
	//chaincode lifecycle commands

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
	allowedCharsVersion       = "[A-Za-z0-9_.+-]+"
)

// Support contains functions that LSCC requires to execute its tasks
type Support interface {
	// PutChaincodeToLocalStorage stores the supplied chaincode
	// package to local storage (i.e. the file system)
	PutChaincodeToLocalStorage(ccprovider.CCPackage) error

	// GetChaincodeFromLocalStorage retrieves the chaincode package
	// for the requested chaincode, specified by name and version
	GetChaincodeFromLocalStorage(ccname string, ccversion string) (ccprovider.CCPackage, error)

	// GetChaincodesFromLocalStorage returns an array of all chaincode
	// data that have previously been persisted to local storage
	GetChaincodesFromLocalStorage() (*pb.ChaincodeQueryResponse, error)

	// GetInstantiationPolicy returns the instantiation policy for the
	// supplied chaincode (or the channel's default if none was specified)
	GetInstantiationPolicy(channel string, ccpack ccprovider.CCPackage) ([]byte, error)

	// CheckInstantiationPolicy checks whether the supplied signed proposal
	// complies with the supplied instantiation policy
	CheckInstantiationPolicy(signedProposal *pb.SignedProposal, chainName string, instantiationPolicy []byte) error
}

//---------- the LSCC -----------------

// LifeCycleSysCC implements chaincode lifecycle and policies around it
type lifeCycleSysCC struct {
	// sccprovider is the interface with which we call
	// methods of the system chaincode package without
	// import cycles
	sccprovider sysccprovider.SystemChaincodeProvider

	// policyChecker is the interface used to perform
	// access control
	policyChecker policy.PolicyChecker

	// support provides the implementation of several
	// static functions
	support Support
}

func NewLifeCycleSysCC() *lifeCycleSysCC {
	return &lifeCycleSysCC{support: &supportImpl{}}
}

//-------------- helper functions ------------------
//create the chaincode on the given chain

//create the chaincode on the given chain
func (lscc *lifeCycleSysCC) putChaincodeData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
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

// putChaincodeCollectionData adds collection data for the chaincode
func (lscc *lifeCycleSysCC) putChaincodeCollectionData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData, collectionConfigBytes []byte) error {
	if cd == nil {
		return errors.New("nil ChaincodeData")
	}

	if len(collectionConfigBytes) == 0 {
		logger.Debug("No collection configuration specified")
		return nil
	}

	collections := &common.CollectionConfigPackage{}
	err := proto.Unmarshal(collectionConfigBytes, collections)
	if err != nil {
		return errors.Errorf("invalid collection configuration supplied for chaincode %s:%s", cd.Name, cd.Version)
	}

	// TODO: FAB-6526 - to add validation of the collections object

	key := privdata.BuildCollectionKVSKey(cd.Name)

	existingCollection, err := stub.GetState(key)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("unable to check whether collection existed earlier for chaincode %s:%s", cd.Name, cd.Version))
	}
	// currently, collections are immutable. Support for collection upgrade will be added later
	if existingCollection != nil {
		return errors.Errorf("collection data should not exist for chaincode %s:%s", cd.Name, cd.Version)
	}

	err = stub.PutState(key, collectionConfigBytes)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("error putting collection for chaincode %s:%s", cd.Name, cd.Version))
	}

	return nil
}

//checks for existence of chaincode on the given channel
func (lscc *lifeCycleSysCC) getCCInstance(stub shim.ChaincodeStubInterface, ccname string) ([]byte, error) {
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
func (lscc *lifeCycleSysCC) getChaincodeData(ccname string, cdbytes []byte) (*ccprovider.ChaincodeData, error) {
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
func (lscc *lifeCycleSysCC) getCCCode(ccname string, cdbytes []byte) (*ccprovider.ChaincodeData, *pb.ChaincodeDeploymentSpec, []byte, error) {
	cd, err := lscc.getChaincodeData(ccname, cdbytes)
	if err != nil {
		return nil, nil, nil, err
	}

	ccpack, err := lscc.support.GetChaincodeFromLocalStorage(ccname, cd.Version)
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
func (lscc *lifeCycleSysCC) getChaincodes(stub shim.ChaincodeStubInterface) pb.Response {
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
		ccpack, err := lscc.support.GetChaincodeFromLocalStorage(ccdata.Name, ccdata.Version)
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
func (lscc *lifeCycleSysCC) getInstalledChaincodes() pb.Response {
	// get chaincode query response proto which contains information about all
	// installed chaincodes
	cqr, err := lscc.support.GetChaincodesFromLocalStorage()
	if err != nil {
		return shim.Error(err.Error())
	}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}

//check validity of chain name
func (lscc *lifeCycleSysCC) isValidChainName(chainname string) bool {
	//TODO we probably need more checks
	if chainname == "" {
		return false
	}
	return true
}

// isValidChaincodeName checks the validity of chaincode name. Chaincode names
// should never be blank and should only consist of alphanumerics, '_', and '-'
func (lscc *lifeCycleSysCC) isValidChaincodeName(chaincodeName string) error {
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
// '+', and '.'
func (lscc *lifeCycleSysCC) isValidChaincodeVersion(chaincodeName string, version string) error {
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

func isValidStatedbArtifactsTar(statedbArtifactsTar []byte) error {

	var dbArtifactsDirFilter = map[string]bool{"META-INF/statedb/couchdb/indexes": true}

	// Extract the metadata files from the archive
	fileEntries, err := ccprovider.ExtractFileEntries(statedbArtifactsTar, dbArtifactsDirFilter)
	if err != nil {
		return err
	}

	// iterate through the files and validate
	for _, fileEntry := range fileEntries {
		indexData := fileEntry.FileContent
		tarDir, filename := filepath.Split(fileEntry.FileHeader.Name)

		// Validation is based on the passed metadata directory, e.g. META-INF/statedb/couchdb/indexes
		// Clean metadata directory to remove trailing slash
		err = ccmetadata.ValidateMetadataFile(filename, indexData, filepath.Clean(tarDir))
		if err != nil {
			return err
		}
	}

	return nil
}

// executeInstall implements the "install" Invoke transaction
func (lscc *lifeCycleSysCC) executeInstall(stub shim.ChaincodeStubInterface, ccbytes []byte) error {
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

	// Get any statedb artifacts from the chaincode package, e.g. couchdb index definitions
	statedbArtifactsTar, err := ccprovider.ExtractStatedbArtifactsFromCCPackage(ccpack)
	if err != nil {
		return err
	}

	if err = isValidStatedbArtifactsTar(statedbArtifactsTar); err != nil {
		return InvalidStatedbArtifactsErr(err.Error())
	}

	chaincodeDefinition := &cceventmgmt.ChaincodeDefinition{
		Name:    ccpack.GetChaincodeData().Name,
		Version: ccpack.GetChaincodeData().Version,
		Hash:    ccpack.GetId()} // Note - The chaincode 'id' is the hash of chaincode's (CodeHash || MetaDataHash), aka fingerprint

	// HandleChaincodeInstall will apply any statedb artifacts (e.g. couchdb indexes) to
	// any channel's statedb where the chaincode is already instantiated
	// Note - this step is done prior to PutChaincodeToLocalStorage() since this step is idempotent and harmless until endorsements start,
	// that is, if there are errors deploying the indexes the chaincode install can safely be re-attempted later.
	err = cceventmgmt.GetMgr().HandleChaincodeInstall(chaincodeDefinition, statedbArtifactsTar)
	if err != nil {
		return err
	}

	// Finally, if everything is good above, install the chaincode to local peer file system so that endorsements can start
	if err = lscc.support.PutChaincodeToLocalStorage(ccpack); err != nil {
		return err
	}

	logger.Infof("Installed Chaincode [%s] Version [%s] to peer", ccpack.GetChaincodeData().Name, ccpack.GetChaincodeData().Version)

	return nil
}

// executeDeployOrUpgrade routes the code path either to executeDeploy or executeUpgrade
// depending on its function argument
func (lscc *lifeCycleSysCC) executeDeployOrUpgrade(
	stub shim.ChaincodeStubInterface,
	chainname string,
	cds *pb.ChaincodeDeploymentSpec,
	policy, escc, vscc, collectionConfigBytes []byte,
	function string,
) (*ccprovider.ChaincodeData, error) {
	if err := lscc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeId.Name); err != nil {
		return nil, err
	}

	if err := lscc.isValidChaincodeVersion(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version); err != nil {
		return nil, err
	}

	ccpack, err := lscc.support.GetChaincodeFromLocalStorage(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version)
	if err != nil {
		retErrMsg := fmt.Sprintf("cannot get package for chaincode (%s:%s)", cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version)
		logger.Errorf("%s-err:%s", retErrMsg, err)
		return nil, fmt.Errorf("%s", retErrMsg)
	}
	cd := ccpack.GetChaincodeData()

	switch function {
	case DEPLOY:
		return lscc.executeDeploy(stub, chainname, cds, policy, escc, vscc, cd, ccpack, collectionConfigBytes)
	case UPGRADE:
		return lscc.executeUpgrade(stub, chainname, cds, policy, escc, vscc, cd, ccpack)
	default:
		logger.Panicf("Programming error, unexpected function '%s'", function)
		panic("") // unreachable code
	}
}

// executeDeploy implements the "instantiate" Invoke transaction
func (lscc *lifeCycleSysCC) executeDeploy(
	stub shim.ChaincodeStubInterface,
	chainname string,
	cds *pb.ChaincodeDeploymentSpec,
	policy []byte,
	escc []byte,
	vscc []byte,
	cdfs *ccprovider.ChaincodeData,
	ccpackfs ccprovider.CCPackage,
	collectionConfigBytes []byte,
) (*ccprovider.ChaincodeData, error) {
	//just test for existence of the chaincode in the LSCC
	_, err := lscc.getCCInstance(stub, cds.ChaincodeSpec.ChaincodeId.Name)
	if err == nil {
		return nil, ExistsErr(cds.ChaincodeSpec.ChaincodeId.Name)
	}

	//retain chaincode specific data and fill channel specific ones
	cdfs.Escc = string(escc)
	cdfs.Vscc = string(vscc)
	cdfs.Policy = policy

	// retrieve and evaluate instantiation policy
	cdfs.InstantiationPolicy, err = lscc.support.GetInstantiationPolicy(chainname, ccpackfs)
	if err != nil {
		return nil, err
	}
	// get the signed instantiation proposal
	signedProp, err := stub.GetSignedProposal()
	if err != nil {
		return nil, err
	}
	err = lscc.support.CheckInstantiationPolicy(signedProp, chainname, cdfs.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	err = lscc.putChaincodeData(stub, cdfs)
	if err != nil {
		return nil, err
	}

	err = lscc.putChaincodeCollectionData(stub, cdfs, collectionConfigBytes)
	if err != nil {
		return nil, err
	}

	return cdfs, nil
}

// executeUpgrade implements the "upgrade" Invoke transaction.
func (lscc *lifeCycleSysCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, cds *pb.ChaincodeDeploymentSpec, policy []byte, escc []byte, vscc []byte, cdfs *ccprovider.ChaincodeData, ccpackfs ccprovider.CCPackage) (*ccprovider.ChaincodeData, error) {
	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name

	// check for existence of chaincode instance only (it has to exist on the channel)
	// we dont care about the old chaincode on the FS. In particular, user may even
	// have deleted it
	cdbytes, _ := lscc.getCCInstance(stub, chaincodeName)
	if cdbytes == nil {
		return nil, NotFoundErr(chaincodeName)
	}

	//we need the cd to compare the version
	cdLedger, err := lscc.getChaincodeData(chaincodeName, cdbytes)
	if err != nil {
		return nil, err
	}

	//do not upgrade if same version
	if cdLedger.Version == cds.ChaincodeSpec.ChaincodeId.Version {
		return nil, IdenticalVersionErr(cds.ChaincodeSpec.ChaincodeId.Name)
	}

	//do not upgrade if instantiation policy is violated
	if cdLedger.InstantiationPolicy == nil {
		return nil, InstantiationPolicyMissing("")
	}
	// get the signed instantiation proposal
	signedProp, err := stub.GetSignedProposal()
	if err != nil {
		return nil, err
	}
	err = lscc.support.CheckInstantiationPolicy(signedProp, chainName, cdLedger.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	//retain chaincode specific data and fill channel specific ones
	cdfs.Escc = string(escc)
	cdfs.Vscc = string(vscc)
	cdfs.Policy = policy

	// retrieve and evaluate new instantiation policy
	cdfs.InstantiationPolicy, err = lscc.support.GetInstantiationPolicy(chainName, ccpackfs)
	if err != nil {
		return nil, err
	}
	err = lscc.support.CheckInstantiationPolicy(signedProp, chainName, cdfs.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	err = lscc.putChaincodeData(stub, cdfs)
	if err != nil {
		return nil, err
	}

	return cdfs, nil
}

//-------------- the chaincode stub interface implementation ----------

//Init only initializes the system chaincode provider
func (lscc *lifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
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
func (lscc *lifeCycleSysCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
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
	case DEPLOY, UPGRADE:
		// we expect a minimum of 3 arguments, the function
		// name, the chain name and deployment spec
		if len(args) < 3 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		//chain the chaincode should be associated with. It
		//should be created with a register call
		chainname := string(args[1])

		if !lscc.isValidChainName(chainname) {
			return shim.Error(InvalidChainNameErr(chainname).Error())
		}

		ac, exists := lscc.sccprovider.GetApplicationConfig(chainname)
		if !exists {
			logger.Panicf("programming error, non-existent appplication config for channel '%s'", chainname)
		}

		// the maximum number of arguments depends on the capability of the channel
		if (!ac.Capabilities().PrivateChannelData() && len(args) > 6) ||
			(ac.Capabilities().PrivateChannelData() && len(args) > 7) {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		depSpec := args[2]
		cds, err := utils.GetChaincodeDeploymentSpec(depSpec)
		if err != nil {
			return shim.Error(err.Error())
		}

		// optional arguments here (they can each be nil and may or may not be present)
		// args[3] is a marshalled SignaturePolicyEnvelope representing the endorsement policy
		// args[4] is the name of escc
		// args[5] is the name of vscc
		// args[6] is a marshalled CollectionConfigPackage struct
		var EP []byte
		if len(args) > 3 && len(args[3]) > 0 {
			EP = args[3]
		} else {
			p := cauthdsl.SignedByAnyMember(peer.GetMSPIDs(chainname))
			EP, err = utils.Marshal(p)
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

		var collectionsConfig []byte
		// we proceed with a non-nil collection configuration only if
		// we support the PrivateChannelData capability
		if ac.Capabilities().PrivateChannelData() && len(args) > 6 {
			collectionsConfig = args[6]
		}

		cd, err := lscc.executeDeployOrUpgrade(stub, chainname, cds, EP, escc, vscc, collectionsConfig, function)
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
		var resource string
		switch function {
		case GETCCINFO:
			resource = resources.LSCC_GETCCINFO
		case GETDEPSPEC:
			resource = resources.LSCC_GETDEPSPEC
		case GETCCDATA:
			resource = resources.LSCC_GETCCDATA
		}
		if err = aclmgmt.GetACLProvider().CheckACL(resource, chain, sp); err != nil {
			return shim.Error(fmt.Sprintf("Authorization request failed %s: %s", chain, err))
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
