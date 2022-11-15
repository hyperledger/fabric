/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"bytes"
	"fmt"
	"regexp"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/internal/ccmetadata"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// The lifecycle system chaincode manages chaincodes deployed
// on this peer. It manages chaincodes via Invoke proposals.
//     "Args":["deploy",<ChaincodeDeploymentSpec>]
//     "Args":["upgrade",<ChaincodeDeploymentSpec>]
//     "Args":["stop",<ChaincodeInvocationSpec>]
//     "Args":["start",<ChaincodeInvocationSpec>]

var (
	logger = flogging.MustGetLogger("lscc")
	// NOTE these regular expressions should stay in sync with those defined in
	// core/chaincode/lifecycle/scc.go until LSCC has been removed.
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
	GetChaincodeFromLocalStorage(ccNameVersion string) (ccprovider.CCPackage, error)

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

type ChaincodeBuilder interface {
	Build(ccid string) error
}

// MSPsIDGetter is used to get the MSP IDs for a channel.
type MSPIDsGetter func(string) []string

// IDMSPManagerGetters used to get the MSP Manager for a channel.
type MSPManagerGetter func(string) msp.MSPManager

//---------- the LSCC -----------------

// SCC implements chaincode lifecycle and policies around it
type SCC struct {
	// aclProvider is responsible for access control evaluation
	ACLProvider aclmgmt.ACLProvider

	BuiltinSCCs scc.BuiltinSCCs

	// SCCProvider is the interface which is passed into system chaincodes
	// to access other parts of the system
	SCCProvider sysccprovider.SystemChaincodeProvider

	// Support provides the implementation of several
	// static functions
	Support FilesystemSupport

	GetMSPIDs MSPIDsGetter

	GetMSPManager MSPManagerGetter

	BuildRegistry *container.BuildRegistry

	ChaincodeBuilder ChaincodeBuilder

	EbMetadataProvider *externalbuilder.MetadataProvider

	// BCCSP instance
	BCCSP bccsp.BCCSP

	PackageCache PackageCache
}

// PeerShim adapts the peer instance for use with LSCC by providing methods
// previously provided by the scc provider.  If the lscc code weren't all getting
// deleted soon, it would probably be worth rewriting it to use these APIs directly
// rather that go through this shim, but it will be gone soon.
type PeerShim struct {
	Peer *peer.Peer
}

// GetQueryExecutorForLedger returns a query executor for the specified channel
func (p *PeerShim) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	l := p.Peer.GetLedger(cid)
	if l == nil {
		return nil, fmt.Errorf("Could not retrieve ledger for channel %s", cid)
	}

	return l.NewQueryExecutor()
}

// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists
func (p *PeerShim) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return p.Peer.GetApplicationConfig(cid)
}

// Returns the policy manager associated to the passed channel
// and whether the policy manager exists
func (p *PeerShim) PolicyManager(channelID string) (policies.Manager, bool) {
	m := p.Peer.GetPolicyManager(channelID)
	return m, (m != nil)
}

func (lscc *SCC) Name() string              { return "lscc" }
func (lscc *SCC) Chaincode() shim.Chaincode { return lscc }

type PackageCache struct {
	Mutex             sync.RWMutex
	ValidatedPackages map[string]*ccprovider.ChaincodeData
}

type LegacySecurity struct {
	Support      FilesystemSupport
	PackageCache *PackageCache
}

func (ls *LegacySecurity) SecurityCheckLegacyChaincode(cd *ccprovider.ChaincodeData) error {
	ccid := cd.ChaincodeID()

	ls.PackageCache.Mutex.RLock()
	fsData, ok := ls.PackageCache.ValidatedPackages[ccid]
	ls.PackageCache.Mutex.RUnlock()

	if !ok {
		ls.PackageCache.Mutex.Lock()
		defer ls.PackageCache.Mutex.Unlock()
		fsData, ok = ls.PackageCache.ValidatedPackages[ccid]
		if !ok {
			ccpack, err := ls.Support.GetChaincodeFromLocalStorage(cd.ChaincodeID())
			if err != nil {
				return InvalidDeploymentSpecErr(err.Error())
			}

			// This is 'the big security check', though it's no clear what's being accomplished
			// here.  Basically, it seems to try to verify that the chaincode definition matches
			// what's on the filesystem, which, might include instantiation policy, but it's
			// not obvious from the code, and was being checked separately, so we check it
			// explicitly below.
			if err = ccpack.ValidateCC(cd); err != nil {
				return InvalidCCOnFSError(err.Error())
			}

			if ls.PackageCache.ValidatedPackages == nil {
				ls.PackageCache.ValidatedPackages = map[string]*ccprovider.ChaincodeData{}
			}

			fsData = ccpack.GetChaincodeData()
			ls.PackageCache.ValidatedPackages[ccid] = fsData
		}
	}

	// we have the info from the fs, check that the policy
	// matches the one on the file system if one was specified;
	// this check is required because the admin of this peer
	// might have specified instantiation policies for their
	// chaincode, for example to make sure that the chaincode
	// is only instantiated on certain channels; a malicious
	// peer on the other hand might have created a deploy
	// transaction that attempts to bypass the instantiation
	// policy. This check is there to ensure that this will not
	// happen, i.e. that the peer will refuse to invoke the
	// chaincode under these conditions. More info on
	// https://jira.hyperledger.org/browse/FAB-3156
	if fsData.InstantiationPolicy != nil {
		if !bytes.Equal(fsData.InstantiationPolicy, cd.InstantiationPolicy) {
			return fmt.Errorf("Instantiation policy mismatch for cc %s", cd.ChaincodeID())
		}
	}

	return nil
}

func (lscc *SCC) ChaincodeEndorsementInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
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

	ls := &LegacySecurity{
		Support:      lscc.Support,
		PackageCache: &lscc.PackageCache,
	}

	err = ls.SecurityCheckLegacyChaincode(chaincodeData)
	if err != nil {
		return nil, errors.WithMessage(err, "failed security checks")
	}

	return &lifecycle.ChaincodeEndorsementInfo{
		Version:           chaincodeData.Version,
		EndorsementPlugin: chaincodeData.Escc,
		ChaincodeID:       chaincodeData.Name + ":" + chaincodeData.Version,
	}, nil
}

// ValidationInfo returns name&arguments of the validation plugin for the supplied chaincode.
// The function returns two types of errors, unexpected errors and validation errors. The
// reason for this is that this function is to be called from the validation code, which
// needs to tell apart the two types of error to halt processing on the channel if the
// unexpected error is not nil and mark the transaction as invalid if the validation error
// is not nil.
func (lscc *SCC) ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error) {
	chaincodeDataBytes, err := qe.GetState("lscc", chaincodeName)
	if err != nil {
		// failure to access the ledger is clearly an unexpected
		// error since we expect the ledger to be reachable
		unexpectedErr = errors.Wrapf(err, "could not retrieve state for chaincode %s", chaincodeName)
		return
	}

	if chaincodeDataBytes == nil {
		// no chaincode definition is a validation error since
		// we're trying to retrieve chaincode definitions for a non-existent chaincode
		validationErr = errors.Errorf("chaincode %s not found", chaincodeName)
		return
	}

	chaincodeData := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(chaincodeDataBytes, chaincodeData)
	if err != nil {
		// this kind of data corruption is unexpected since our code
		// always marshals ChaincodeData into these keys
		unexpectedErr = errors.Wrapf(err, "chaincode %s has bad definition", chaincodeName)
		return
	}

	plugin = chaincodeData.Vscc
	args = chaincodeData.Policy
	return
}

// create the chaincode on the given chain
func (lscc *SCC) putChaincodeData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData) error {
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
func checkCollectionMemberPolicy(collectionConfig *pb.CollectionConfig, mspmgr msp.MSPManager) error {
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
func (lscc *SCC) putChaincodeCollectionData(stub shim.ChaincodeStubInterface, cd *ccprovider.ChaincodeData, collectionConfigBytes []byte) error {
	if cd == nil {
		return errors.New("nil ChaincodeData")
	}

	if len(collectionConfigBytes) == 0 {
		logger.Debug("No collection configuration specified")
		return nil
	}

	collections := &pb.CollectionConfigPackage{}
	err := proto.Unmarshal(collectionConfigBytes, collections)
	if err != nil {
		return errors.Errorf("invalid collection configuration supplied for chaincode %s:%s", cd.Name, cd.Version)
	}

	mspmgr := lscc.GetMSPManager(stub.GetChannelID())
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
		return errors.WithMessagef(err, "error putting collection for chaincode %s:%s", cd.Name, cd.Version)
	}

	return nil
}

// getChaincodeCollectionData retrieve collections config.
func (lscc *SCC) getChaincodeCollectionData(stub shim.ChaincodeStubInterface, chaincodeName string) pb.Response {
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

// checks for existence of chaincode on the given channel
func (lscc *SCC) getCCInstance(stub shim.ChaincodeStubInterface, ccname string) ([]byte, error) {
	cdbytes, err := stub.GetState(ccname)
	if err != nil {
		return nil, TXNotFoundErr(err.Error())
	}
	if cdbytes == nil {
		return nil, NotFoundErr(ccname)
	}

	return cdbytes, nil
}

// gets the cd out of the bytes
func (lscc *SCC) getChaincodeData(ccname string, cdbytes []byte) (*ccprovider.ChaincodeData, error) {
	cd := &ccprovider.ChaincodeData{}
	err := proto.Unmarshal(cdbytes, cd)
	if err != nil {
		return nil, MarshallErr(ccname)
	}

	// this should not happen but still a sanity check is not a bad thing
	if cd.Name != ccname {
		return nil, ChaincodeMismatchErr(fmt.Sprintf("%s!=%s", ccname, cd.Name))
	}

	return cd, nil
}

// checks for existence of chaincode on the given chain
func (lscc *SCC) getCCCode(ccname string, cdbytes []byte) (*pb.ChaincodeDeploymentSpec, []byte, error) {
	cd, err := lscc.getChaincodeData(ccname, cdbytes)
	if err != nil {
		return nil, nil, err
	}

	ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(cd.ChaincodeID())
	if err != nil {
		return nil, nil, InvalidDeploymentSpecErr(err.Error())
	}

	// this is the big test and the reason every launch should go through
	// getChaincode call. We validate the chaincode entry against the
	// chaincode in FS
	if err = ccpack.ValidateCC(cd); err != nil {
		return nil, nil, InvalidCCOnFSError(err.Error())
	}

	// these are guaranteed to be non-nil because we got a valid ccpack
	depspec := ccpack.GetDepSpec()
	depspecbytes := ccpack.GetDepSpecBytes()

	return depspec, depspecbytes, nil
}

// getChaincodes returns all chaincodes instantiated on this LSCC's channel
func (lscc *SCC) getChaincodes(stub shim.ChaincodeStubInterface) pb.Response {
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
		ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(ccdata.ChaincodeID())
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
func (lscc *SCC) getInstalledChaincodes() pb.Response {
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
func (lscc *SCC) isValidChannelName(channel string) bool {
	// TODO we probably need more checks
	return channel != ""
}

// isValidChaincodeName checks the validity of chaincode name. Chaincode names
// should never be blank and should only consist of alphanumerics, '_', and '-'
func (lscc *SCC) isValidChaincodeName(chaincodeName string) error {
	if !ChaincodeNameRegExp.MatchString(chaincodeName) {
		return InvalidChaincodeNameErr(chaincodeName)
	}

	return nil
}

// isValidChaincodeVersion checks the validity of chaincode version. Versions
// should never be blank and should only consist of alphanumerics, '_',  '-',
// '+', and '.'
func (lscc *SCC) isValidChaincodeVersion(chaincodeName string, version string) error {
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
func (lscc *SCC) executeInstall(stub shim.ChaincodeStubInterface, ccbytes []byte) error {
	ccpack, err := ccprovider.GetCCPackage(ccbytes, lscc.BCCSP)
	if err != nil {
		return err
	}

	cds := ccpack.GetDepSpec()

	if cds == nil {
		return fmt.Errorf("nil deployment spec from the CC package")
	}

	if err = lscc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeId.Name); err != nil {
		return err
	}

	if err = lscc.isValidChaincodeVersion(cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version); err != nil {
		return err
	}

	if lscc.BuiltinSCCs.IsSysCC(cds.ChaincodeSpec.ChaincodeId.Name) {
		return errors.Errorf("cannot install: %s is the name of a system chaincode", cds.ChaincodeSpec.ChaincodeId.Name)
	}

	// We must put the chaincode package on the filesystem prior to building it.
	// This creates a race for endorsements coming in and executing the chaincode
	// prior to indexes being installed, but, such is life, and this case is already
	// present when an already installed chaincode is instantiated.
	if err = lscc.Support.PutChaincodeToLocalStorage(ccpack); err != nil {
		return err
	}

	ccid := ccpack.GetChaincodeData().Name + ":" + ccpack.GetChaincodeData().Version
	buildStatus, building := lscc.BuildRegistry.BuildStatus(ccid)
	if !building {
		err := lscc.ChaincodeBuilder.Build(ccid)
		buildStatus.Notify(err)
	}
	<-buildStatus.Done()
	if err := buildStatus.Err(); err != nil {
		return errors.WithMessage(err, "chaincode installed to peer but could not build chaincode")
	}

	md, err := lscc.EbMetadataProvider.PackageMetadata(ccid)
	if err != nil {
		return errors.WithMessage(err, "external builder release metadata found, but could not be packaged")
	}

	if md == nil {
		// Get any statedb artifacts from the chaincode package, e.g. couchdb index definitions
		md, err = ccprovider.ExtractStatedbArtifactsFromCCPackage(ccpack)
		if err != nil {
			return err
		}
	}

	if err = isValidStatedbArtifactsTar(md); err != nil {
		return InvalidStatedbArtifactsErr(err.Error())
	}

	chaincodeDefinition := &cceventmgmt.ChaincodeDefinition{
		Name:    ccpack.GetChaincodeData().Name,
		Version: ccpack.GetChaincodeData().Version,
		Hash:    ccpack.GetId(),
	} // Note - The chaincode 'id' is the hash of chaincode's (CodeHash || MetaDataHash), aka fingerprint

	// HandleChaincodeInstall will apply any statedb artifacts (e.g. couchdb indexes) to
	// any channel's statedb where the chaincode is already instantiated
	// Note - this step is done prior to PutChaincodeToLocalStorage() since this step is idempotent and harmless until endorsements start,
	// that is, if there are errors deploying the indexes the chaincode install can safely be re-attempted later.
	err = cceventmgmt.GetMgr().HandleChaincodeInstall(chaincodeDefinition, md)
	defer func() {
		cceventmgmt.GetMgr().ChaincodeInstallDone(err == nil)
	}()
	if err != nil {
		return err
	}

	logger.Infof("Installed Chaincode [%s] Version [%s] to peer", ccpack.GetChaincodeData().Name, ccpack.GetChaincodeData().Version)

	return nil
}

// executeDeployOrUpgrade routes the code path either to executeDeploy or executeUpgrade
// depending on its function argument
func (lscc *SCC) executeDeployOrUpgrade(
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

	chaincodeNameVersion := chaincodeName + ":" + chaincodeVersion

	ccpack, err := lscc.Support.GetChaincodeFromLocalStorage(chaincodeNameVersion)
	if err != nil {
		retErrMsg := fmt.Sprintf("cannot get package for chaincode (%s)", chaincodeNameVersion)
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
func (lscc *SCC) executeDeploy(
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
	// just test for existence of the chaincode in the LSCC
	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name
	_, err := lscc.getCCInstance(stub, chaincodeName)
	if err == nil {
		return nil, ExistsErr(chaincodeName)
	}

	// retain chaincode specific data and fill channel specific ones
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
func (lscc *SCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, cds *pb.ChaincodeDeploymentSpec, policy []byte, escc []byte, vscc []byte, cdfs *ccprovider.ChaincodeData, ccpackfs ccprovider.CCPackage, collectionConfigBytes []byte) (*ccprovider.ChaincodeData, error) {
	chaincodeName := cds.ChaincodeSpec.ChaincodeId.Name

	// check for existence of chaincode instance only (it has to exist on the channel)
	// we dont care about the old chaincode on the FS. In particular, user may even
	// have deleted it
	cdbytes, _ := lscc.getCCInstance(stub, chaincodeName)
	if cdbytes == nil {
		return nil, NotFoundErr(chaincodeName)
	}

	// we need the cd to compare the version
	cdLedger, err := lscc.getChaincodeData(chaincodeName, cdbytes)
	if err != nil {
		return nil, err
	}

	// do not upgrade if same version
	if cdLedger.Version == cds.ChaincodeSpec.ChaincodeId.Version {
		return nil, IdenticalVersionErr(chaincodeName)
	}

	// do not upgrade if instantiation policy is violated
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

	// retain chaincode specific data and fill channel specific ones
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
	lifecycleEventBytes := protoutil.MarshalOrPanic(lifecycleEvent)
	stub.SetEvent(UPGRADE, lifecycleEventBytes)
	return cdfs, nil
}

//-------------- the chaincode stub interface implementation ----------

// Init is mostly useless for SCC
func (lscc *SCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Invoke implements lifecycle functions "deploy", "start", "stop", "upgrade".
// Deploy's arguments -  {[]byte("deploy"), []byte(<chainname>), <unmarshalled pb.ChaincodeDeploymentSpec>}
//
// Invoke also implements some query-like functions
// Get chaincode arguments -  {[]byte("getid"), []byte(<chainname>), []byte(<chaincodename>)}
func (lscc *SCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
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

		// 2. check install policy
		if err = lscc.ACLProvider.CheckACL(resources.Lscc_Install, "", sp); err != nil {
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

		if ac.Capabilities().LifecycleV20() {
			return shim.Error(fmt.Sprintf("Channel '%s' has been migrated to the new lifecycle, LSCC is now read-only", channel))
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
			return shim.Error(fmt.Sprintf("error unmarshalling ChaincodeDeploymentSpec: %s", err))
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
			mspIDs := lscc.GetMSPIDs(channel)
			p := policydsl.SignedByAnyMember(mspIDs)
			EP, err = protoutil.Marshal(p)
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

		// 2. check Lscc_GetInstalledChaincodes policy
		if err = lscc.ACLProvider.CheckACL(resources.Lscc_GetInstalledChaincodes, "", sp); err != nil {
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
