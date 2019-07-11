/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"fmt"
	"regexp"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/policyprovider"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
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

var (
	logger                 = flogging.MustGetLogger("lscc")
	ChaincodeNameRegExp    = regexp.MustCompile("^[a-zA-Z0-9]+([-_][a-zA-Z0-9]+)*$")
	ChaincodeVersionRegExp = regexp.MustCompile("^[A-Za-z0-9_.+-]+$")
)

const (
	// chaincode lifecycle commands

	// INSTALL install command
	INSTALL = "install"

	// DEPLOY deploy command
	DEPLOY = "deploy"

	// UPGRADE upgrade chaincode
	UPGRADE = "upgrade"

	// CCEXISTS get chaincode
	CCEXISTS = "getid"

	// CHAINCODEEXISTS get chaincode alias
	CHAINCODEEXISTS = "ChaincodeExists"

	// GETDEPSPEC get ChaincodeDeploymentSpec
	GETDEPSPEC = "getdepspec"

	// GETDEPLOYMENTSPEC get ChaincodeDeploymentSpec alias
	GETDEPLOYMENTSPEC = "GetDeploymentSpec"

	// GETCCDATA get ChaincodeData
	GETCCDATA = "getccdata"

	// GETCHAINCODEDATA get ChaincodeData alias
	GETCHAINCODEDATA = "GetChaincodeData"

	// GETCHAINCODES gets the instantiated chaincodes on a channel
	GETCHAINCODES = "getchaincodes"

	// GETCHAINCODESALIAS gets the instantiated chaincodes on a channel
	GETCHAINCODESALIAS = "GetChaincodes"

	// GETINSTALLEDCHAINCODES gets the installed chaincodes on a peer
	GETINSTALLEDCHAINCODES = "getinstalledchaincodes"

	// GETINSTALLEDCHAINCODESALIAS gets the installed chaincodes on a peer
	GETINSTALLEDCHAINCODESALIAS = "GetInstalledChaincodes"

	// GETCOLLECTIONSCONFIG gets the collections config for a chaincode
	GETCOLLECTIONSCONFIG = "GetCollectionsConfig"

	// GETCOLLECTIONSCONFIGALIAS gets the collections config for a chaincode
	GETCOLLECTIONSCONFIGALIAS = "getcollectionsconfig"
)

// FilesystemSupport contains functions that LSCC requires to execute its tasks
type FilesystemSupport interface {
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
type LifeCycleSysCC struct {
	// aclProvider is responsible for access control evaluation
	ACLProvider aclmgmt.ACLProvider

	// SCCProvider is the interface which is passed into system chaincodes
	// to access other parts of the system
	SCCProvider sysccprovider.SystemChaincodeProvider

	// PolicyChecker is the interface used to perform
	// access control
	PolicyChecker policy.PolicyChecker

	// Support provides the implementation of several
	// static functions
	Support FilesystemSupport

	PlatformRegistry *platforms.Registry
}

// New creates a new instance of the LSCC
// Typically there is only one of these per peer
func New(sccp sysccprovider.SystemChaincodeProvider, ACLProvider aclmgmt.ACLProvider, platformRegistry *platforms.Registry) *LifeCycleSysCC {
	return &LifeCycleSysCC{
		Support:          &supportImpl{},
		PolicyChecker:    policyprovider.GetPolicyChecker(),
		SCCProvider:      sccp,
		ACLProvider:      ACLProvider,
		PlatformRegistry: platformRegistry,
	}
}

func (lscc *LifeCycleSysCC) Name() string              { return "lscc" }
func (lscc *LifeCycleSysCC) Path() string              { return "github.com/hyperledger/fabric/core/scc/lscc" }
func (lscc *LifeCycleSysCC) InitArgs() [][]byte        { return nil }
func (lscc *LifeCycleSysCC) Chaincode() shim.Chaincode { return lscc }
func (lscc *LifeCycleSysCC) InvokableExternal() bool   { return true }
func (lscc *LifeCycleSysCC) InvokableCC2CC() bool      { return true }
func (lscc *LifeCycleSysCC) Enabled() bool             { return true }

func (lscc *LifeCycleSysCC) ChaincodeContainerInfo(chaincodeName string, qe ledger.QueryExecutor) (*ccprovider.ChaincodeContainerInfo, error) {
	chaincodeDataBytes, err := qe.GetState("lscc", chaincodeName)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve state for chaincode %s", chaincodeName)
	}

	if chaincodeDataBytes == nil {
		return nil, errors.Errorf("chaincode %s not found", chaincodeName)
	}

	// Note, although it looks very tempting to replace the bulk of this function with
	// the below 'ChaincodeDefinition' call, the 'getCCCode' call provides us security
	// by side-effect, so we must leave it as is for now.
	cds, _, err := lscc.getCCCode(chaincodeName, chaincodeDataBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get chaincode code")
	}

	return ccprovider.DeploymentSpecToChaincodeContainerInfo(cds), nil
}

func (lscc *LifeCycleSysCC) ChaincodeDefinition(chaincodeName string, qe ledger.QueryExecutor) (ccprovider.ChaincodeDefinition, error) {
	chaincodeDataBytes, err := qe.GetState("lscc", chaincodeName)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve state for chaincode %s", chaincodeName)
	}

	if chaincodeDataBytes == nil {
		return nil, errors.Errorf("chaincode %s not found", chaincodeName)
	}

	chaincodeData := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(chaincodeDataBytes, chaincodeData)
	if err != nil {
		return nil, errors.Wrapf(err, "chaincode %s has bad definition", chaincodeName)
	}

	return chaincodeData, nil
}

//create the chaincode on the given chain
func (lscc *LifeCycleSysCC) putChaincodeData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
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

// checkCollectionMemberPolicy checks whether the supplied collection configuration
// complies to the given msp configuration and performs semantic validation.
// Channel config may change afterwards (i.e., after endorsement or commit of this transaction).
// Fabric will deal with the situation where some collection configs are no longer meaningful.
// Therefore, the use of channel config for verifying during endorsement is more
// towards catching manual errors in the config as oppose to any attempt of serializability.
func checkCollectionMemberPolicy(collectionConfig *common.CollectionConfig, mspmgr msp.MSPManager) error {
	if mspmgr == nil {
		return fmt.Errorf("msp manager not set")
	}
	msps, err := mspmgr.GetMSPs()
	if err != nil {
		return errors.Wrapf(err, "error getting channel msp")
	}
	if collectionConfig == nil {
		return fmt.Errorf("collection configuration is not set")
	}
	coll := collectionConfig.GetStaticCollectionConfig()
	if coll == nil {
		return fmt.Errorf("collection configuration is empty")
	}
	if coll.MemberOrgsPolicy == nil {
		return fmt.Errorf("collection member policy is not set")
	}
	if coll.MemberOrgsPolicy.GetSignaturePolicy() == nil {
		return fmt.Errorf("collection member org policy is empty")
	}
	// make sure that the orgs listed are actually part of the channel
	// check all principals in the signature policy
	for _, principal := range coll.MemberOrgsPolicy.GetSignaturePolicy().Identities {
		found := false
		var orgID string
		// the member org policy only supports certain principal types
		switch principal.PrincipalClassification {

		case mb.MSPPrincipal_ROLE:
			msprole := &mb.MSPRole{}
			err := proto.Unmarshal(principal.Principal, msprole)
			if err != nil {
				return errors.Wrapf(err, "collection-name: %s -- cannot unmarshal identities", coll.GetName())
			}
			orgID = msprole.MspIdentifier
			// the msp map is indexed using msp IDs - this behavior is implementation specific, making the following check a bit of a hack
			for mspid := range msps {
				if mspid == orgID {
					found = true
					break
				}
			}

		case mb.MSPPrincipal_ORGANIZATION_UNIT:
			mspou := &mb.OrganizationUnit{}
			err := proto.Unmarshal(principal.Principal, mspou)
			if err != nil {
				return errors.Wrapf(err, "collection-name: %s -- cannot unmarshal identities", coll.GetName())
			}
			orgID = mspou.MspIdentifier
			// the msp map is indexed using msp IDs - this behavior is implementation specific, making the following check a bit of a hack
			for mspid := range msps {
				if mspid == orgID {
					found = true
					break
				}
			}

		case mb.MSPPrincipal_IDENTITY:
			orgID = "identity principal"
			for _, msp := range msps {
				_, err := msp.DeserializeIdentity(principal.Principal)
				if err == nil {
					found = true
					break
				}
			}
		default:
			return fmt.Errorf("collection-name: %s -- principal type %v is not supported", coll.GetName(), principal.PrincipalClassification)
		}
		if !found {
			logger.Warningf("collection-name: %s collection member %s is not part of the channel", coll.GetName(), orgID)
		}
	}

	// Call the constructor for SignaturePolicyEnvelope evaluators to perform extra semantic validation.
	// Among other things, this validation catches any out-of-range references to the identities array.
	policyProvider := &cauthdsl.EnvelopeBasedPolicyProvider{Deserializer: mspmgr}
	if _, err := policyProvider.NewPolicy(coll.MemberOrgsPolicy.GetSignaturePolicy()); err != nil {
		logger.Errorf("Invalid member org policy for collection '%s', error: %s", coll.Name, err)
		return errors.WithMessage(err, fmt.Sprintf("invalid member org policy for collection '%s'", coll.Name))
	}

	return nil
}

// putChaincodeCollectionData adds collection data for the chaincode
func (lscc *LifeCycleSysCC) putChaincodeCollectionData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData, collectionConfigBytes []byte) error {
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

	mspmgr := mgmt.GetManagerForChain(stub.GetChannelID())
	if mspmgr == nil {
		return fmt.Errorf("could not get MSP manager for channel %s", stub.GetChannelID())
	}
	for _, collectionConfig := range collections.Config {
		err = checkCollectionMemberPolicy(collectionConfig, mspmgr)
		if err != nil {
			return errors.Wrapf(err, "collection member policy check failed")
		}
	}

	key := privdata.BuildCollectionKVSKey(cd.Name)

	err = stub.PutState(key, collectionConfigBytes)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("error putting collection for chaincode %s:%s", cd.Name, cd.Version))
	}

	return nil
}

// getChaincodeCollectionData retrieve collections config.
func (lscc *LifeCycleSysCC) getChaincodeCollectionData(stub shim.ChaincodeStubInterface, chaincodeName string) pb.Response {
	key := privdata.BuildCollectionKVSKey(chaincodeName)
	collectionsConfigBytes, err := stub.GetState(key)
	if err != nil {
		return shim.Error(err.Error())
	}
	if len(collectionsConfigBytes) == 0 {
		return shim.Error(fmt.Sprintf("collections config not defined for chaincode %s", chaincodeName))
	}
	return shim.Success(collectionsConfigBytes)
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
func (lscc *LifeCycleSysCC) getCCCode(ccname string, cdbytes []byte) (*pb.ChaincodeDeploymentSpec, []byte, error) {
	cd, err := lscc.getChaincodeData(ccname, cdbytes)
	if err != nil {
		return nil, nil, err
	}

	ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(ccname, cd.Version)
	if err != nil {
		return nil, nil, InvalidDeploymentSpecErr(err.Error())
	}

	//this is the big test and the reason every launch should go through
	//getChaincode call. We validate the chaincode entry against the
	//the chaincode in FS
	if err = ccpack.ValidateCC(cd); err != nil {
		return nil, nil, InvalidCCOnFSError(err.Error())
	}

	//these are guaranteed to be non-nil because we got a valid ccpack
	depspec := ccpack.GetDepSpec()
	depspecbytes := ccpack.GetDepSpecBytes()

	return depspec, depspecbytes, nil
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

		// CollectionConfig isn't ChaincodeData
		if privdata.IsCollectionConfigKey(response.Key) {
			continue
		}

		ccdata := &ccprovider.ChaincodeData{}
		if err = proto.Unmarshal(response.Value, ccdata); err != nil {
			return shim.Error(err.Error())
		}

		var path string
		var input string

		// if chaincode is not installed on the system we won't have
		// data beyond name and version
		ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(ccdata.Name, ccdata.Version)
		if err == nil {
			path = ccpack.GetDepSpec().GetChaincodeSpec().ChaincodeId.Path
			input = ccpack.GetDepSpec().GetChaincodeSpec().Input.String()
		}

		// add this specific chaincode's metadata to the array of all chaincodes
		ccInfo := &pb.ChaincodeInfo{Name: ccdata.Name, Version: ccdata.Version, Path: path, Input: input, Escc: ccdata.Escc, Vscc: ccdata.Vscc}
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
	cqr, err := lscc.Support.GetChaincodesFromLocalStorage()
	if err != nil {
		return shim.Error(err.Error())
	}

	cqrbytes, err := proto.Marshal(cqr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(cqrbytes)
}

// check validity of channel name
func (lscc *LifeCycleSysCC) isValidChannelName(channel string) bool {
	// TODO we probably need more checks
	if channel == "" {
		return false
	}
	return true
}

// isValidChaincodeName checks the validity of chaincode name. Chaincode names
// should never be blank and should only consist of alphanumerics, '_', and '-'
func (lscc *LifeCycleSysCC) isValidChaincodeName(chaincodeName string) error {
	if !ChaincodeNameRegExp.MatchString(chaincodeName) {
		return InvalidChaincodeNameErr(chaincodeName)
	}

	return nil
}

// isValidChaincodeVersion checks the validity of chaincode version. Versions
// should never be blank and should only consist of alphanumerics, '_',  '-',
// '+', and '.'
func (lscc *LifeCycleSysCC) isValidChaincodeVersion(chaincodeName string, version string) error {
	if !ChaincodeVersionRegExp.MatchString(version) {
		return InvalidVersionErr(version)
	}

	return nil
}

func isValidStatedbArtifactsTar(statedbArtifactsTar []byte) error {
	// Extract the metadata files from the archive
	// Passing an empty string for the databaseType will validate all artifacts in
	// the archive
	archiveFiles, err := ccprovider.ExtractFileEntries(statedbArtifactsTar, "")
	if err != nil {
		return err
	}
	// iterate through the files and validate
	for _, archiveDirectoryFiles := range archiveFiles {
		for _, fileEntry := range archiveDirectoryFiles {
			indexData := fileEntry.FileContent
			// Validation is based on the passed file name, e.g. META-INF/statedb/couchdb/indexes/indexname.json
			err = ccmetadata.ValidateMetadataFile(fileEntry.FileHeader.Name, indexData)
			if err != nil {
				return err
			}
		}
	}

	return nil
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

	if lscc.SCCProvider.IsSysCC(cds.ChaincodeSpec.ChaincodeId.Name) {
		return errors.Errorf("cannot install: %s is the name of a system chaincode", cds.ChaincodeSpec.ChaincodeId.Name)
	}

	// Get any statedb artifacts from the chaincode package, e.g. couchdb index definitions
	statedbArtifactsTar, err := ccprovider.ExtractStatedbArtifactsFromCCPackage(ccpack, lscc.PlatformRegistry)
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
	defer func() {
		cceventmgmt.GetMgr().ChaincodeInstallDone(err == nil)
	}()
	if err != nil {
		return err
	}

	// Finally, if everything is good above, install the chaincode to local peer file system so that endorsements can start
	if err = lscc.Support.PutChaincodeToLocalStorage(ccpack); err != nil {
		return err
	}

	logger.Infof("Installed Chaincode [%s] Version [%s] to peer", ccpack.GetChaincodeData().Name, ccpack.GetChaincodeData().Version)

	return nil
}

// executeDeployOrUpgrade routes the code path either to executeDeploy or executeUpgrade
// depending on its function argument
func (lscc *LifeCycleSysCC) executeDeployOrUpgrade(
	stub shim.ChaincodeStubInterface,
	chainname string,
	cds *pb.ChaincodeDeploymentSpec,
	policy, escc, vscc, collectionConfigBytes []byte,
	function string,
) (*ccprovider.ChaincodeData, error) {

	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name
	chaincodeVersion := cds.ChaincodeSpec.ChaincodeId.Version

	if err := lscc.isValidChaincodeName(chaincodeName); err != nil {
		return nil, err
	}

	if err := lscc.isValidChaincodeVersion(chaincodeName, chaincodeVersion); err != nil {
		return nil, err
	}

	ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(chaincodeName, chaincodeVersion)
	if err != nil {
		retErrMsg := fmt.Sprintf("cannot get package for chaincode (%s:%s)", chaincodeName, chaincodeVersion)
		logger.Errorf("%s-err:%s", retErrMsg, err)
		return nil, fmt.Errorf("%s", retErrMsg)
	}
	cd := ccpack.GetChaincodeData()

	switch function {
	case DEPLOY:
		return lscc.executeDeploy(stub, chainname, cds, policy, escc, vscc, cd, ccpack, collectionConfigBytes)
	case UPGRADE:
		return lscc.executeUpgrade(stub, chainname, cds, policy, escc, vscc, cd, ccpack, collectionConfigBytes)
	default:
		logger.Panicf("Programming error, unexpected function '%s'", function)
		panic("") // unreachable code
	}
}

// executeDeploy implements the "instantiate" Invoke transaction
func (lscc *LifeCycleSysCC) executeDeploy(
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
	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name
	_, err := lscc.getCCInstance(stub, chaincodeName)
	if err == nil {
		return nil, ExistsErr(chaincodeName)
	}

	//retain chaincode specific data and fill channel specific ones
	cdfs.Escc = string(escc)
	cdfs.Vscc = string(vscc)
	cdfs.Policy = policy

	// retrieve and evaluate instantiation policy
	cdfs.InstantiationPolicy, err = lscc.Support.GetInstantiationPolicy(chainname, ccpackfs)
	if err != nil {
		return nil, err
	}
	// get the signed instantiation proposal
	signedProp, err := stub.GetSignedProposal()
	if err != nil {
		return nil, err
	}
	err = lscc.Support.CheckInstantiationPolicy(signedProp, chainname, cdfs.InstantiationPolicy)
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
func (lscc *LifeCycleSysCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, cds *pb.ChaincodeDeploymentSpec, policy []byte, escc []byte, vscc []byte, cdfs *ccprovider.ChaincodeData, ccpackfs ccprovider.CCPackage, collectionConfigBytes []byte) (*ccprovider.ChaincodeData, error) {

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
		return nil, IdenticalVersionErr(chaincodeName)
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
	err = lscc.Support.CheckInstantiationPolicy(signedProp, chainName, cdLedger.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	//retain chaincode specific data and fill channel specific ones
	cdfs.Escc = string(escc)
	cdfs.Vscc = string(vscc)
	cdfs.Policy = policy

	// retrieve and evaluate new instantiation policy
	cdfs.InstantiationPolicy, err = lscc.Support.GetInstantiationPolicy(chainName, ccpackfs)
	if err != nil {
		return nil, err
	}
	err = lscc.Support.CheckInstantiationPolicy(signedProp, chainName, cdfs.InstantiationPolicy)
	if err != nil {
		return nil, err
	}

	err = lscc.putChaincodeData(stub, cdfs)
	if err != nil {
		return nil, err
	}

	ac, exists := lscc.SCCProvider.GetApplicationConfig(chainName)
	if !exists {
		logger.Panicf("programming error, non-existent appplication config for channel '%s'", chainName)
	}

	if ac.Capabilities().CollectionUpgrade() {
		err = lscc.putChaincodeCollectionData(stub, cdfs, collectionConfigBytes)
		if err != nil {
			return nil, err
		}
	} else {
		if collectionConfigBytes != nil {
			return nil, errors.New(CollectionsConfigUpgradesNotAllowed("").Error())
		}
	}

	lifecycleEvent := &pb.LifecycleEvent{ChaincodeName: chaincodeName}
	lifecycleEventBytes := utils.MarshalOrPanic(lifecycleEvent)
	stub.SetEvent(UPGRADE, lifecycleEventBytes)
	return cdfs, nil
}

//-------------- the chaincode stub interface implementation ----------

//Init is mostly useless for SCC
func (lscc *LifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
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
		if err = lscc.PolicyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s]: %s", function, err))
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

		// channel the chaincode should be associated with. It
		// should be created with a register call
		channel := string(args[1])

		if !lscc.isValidChannelName(channel) {
			return shim.Error(InvalidChannelNameErr(channel).Error())
		}

		ac, exists := lscc.SCCProvider.GetApplicationConfig(channel)
		if !exists {
			logger.Panicf("programming error, non-existent appplication config for channel '%s'", channel)
		}

		// the maximum number of arguments depends on the capability of the channel
		if !ac.Capabilities().PrivateChannelData() && len(args) > 6 {
			return shim.Error(PrivateChannelDataNotAvailable("").Error())
		}
		if ac.Capabilities().PrivateChannelData() && len(args) > 7 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		depSpec := args[2]
		cds := &pb.ChaincodeDeploymentSpec{}
		err := proto.Unmarshal(depSpec, cds)
		if err != nil {
			return shim.Error(fmt.Sprintf("error unmarshaling ChaincodeDeploymentSpec: %s", err))
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
			p := cauthdsl.SignedByAnyMember(peer.GetMSPIDs(channel))
			EP, err = utils.Marshal(p)
			if err != nil {
				return shim.Error(err.Error())
			}
		}

		var escc []byte
		if len(args) > 4 && len(args[4]) > 0 {
			escc = args[4]
		} else {
			escc = []byte("escc")
		}

		var vscc []byte
		if len(args) > 5 && len(args[5]) > 0 {
			vscc = args[5]
		} else {
			vscc = []byte("vscc")
		}

		var collectionsConfig []byte
		// we proceed with a non-nil collection configuration only if
		// we Support the PrivateChannelData capability
		if ac.Capabilities().PrivateChannelData() && len(args) > 6 {
			collectionsConfig = args[6]
		}

		cd, err := lscc.executeDeployOrUpgrade(stub, channel, cds, EP, escc, vscc, collectionsConfig, function)
		if err != nil {
			return shim.Error(err.Error())
		}
		cdbytes, err := proto.Marshal(cd)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(cdbytes)
	case CCEXISTS, CHAINCODEEXISTS, GETDEPSPEC, GETDEPLOYMENTSPEC, GETCCDATA, GETCHAINCODEDATA:
		if len(args) != 3 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		channel := string(args[1])
		ccname := string(args[2])

		// 2. check policy for ACL resource
		var resource string
		switch function {
		case CCEXISTS, CHAINCODEEXISTS:
			resource = resources.Lscc_ChaincodeExists
		case GETDEPSPEC, GETDEPLOYMENTSPEC:
			resource = resources.Lscc_GetDeploymentSpec
		case GETCCDATA, GETCHAINCODEDATA:
			resource = resources.Lscc_GetChaincodeData
		}
		if err = lscc.ACLProvider.CheckACL(resource, channel, sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: %s", function, channel, err))
		}

		cdbytes, err := lscc.getCCInstance(stub, ccname)
		if err != nil {
			logger.Errorf("error getting chaincode %s on channel [%s]: %s", ccname, channel, err)
			return shim.Error(err.Error())
		}

		switch function {
		case CCEXISTS, CHAINCODEEXISTS:
			cd, err := lscc.getChaincodeData(ccname, cdbytes)
			if err != nil {
				return shim.Error(err.Error())
			}
			return shim.Success([]byte(cd.Name))
		case GETCCDATA, GETCHAINCODEDATA:
			return shim.Success(cdbytes)
		case GETDEPSPEC, GETDEPLOYMENTSPEC:
			_, depspecbytes, err := lscc.getCCCode(ccname, cdbytes)
			if err != nil {
				return shim.Error(err.Error())
			}
			return shim.Success(depspecbytes)
		default:
			panic("unreachable")
		}
	case GETCHAINCODES, GETCHAINCODESALIAS:
		if len(args) != 1 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		if err = lscc.ACLProvider.CheckACL(resources.Lscc_GetInstantiatedChaincodes, stub.GetChannelID(), sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s][%s]: %s", function, stub.GetChannelID(), err))
		}

		return lscc.getChaincodes(stub)
	case GETINSTALLEDCHAINCODES, GETINSTALLEDCHAINCODESALIAS:
		if len(args) != 1 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		// 2. check local MSP Admins policy
		if err = lscc.PolicyChecker.CheckPolicyNoChannel(mgmt.Admins, sp); err != nil {
			return shim.Error(fmt.Sprintf("access denied for [%s]: %s", function, err))
		}

		return lscc.getInstalledChaincodes()
	case GETCOLLECTIONSCONFIG, GETCOLLECTIONSCONFIGALIAS:
		if len(args) != 2 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		chaincodeName := string(args[1])

		logger.Debugf("GetCollectionsConfig, chaincodeName:%s, start to check ACL for current identity policy", chaincodeName)
		if err = lscc.ACLProvider.CheckACL(resources.Lscc_GetCollectionsConfig, stub.GetChannelID(), sp); err != nil {
			logger.Debugf("ACL Check Failed for channel:%s, chaincode:%s", stub.GetChannelID(), chaincodeName)
			return shim.Error(fmt.Sprintf("access denied for [%s]: %s", function, err))
		}

		return lscc.getChaincodeCollectionData(stub, chaincodeName)
	}

	return shim.Error(InvalidFunctionErr(function).Error())
}
