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

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

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

// MSPIDsGetter is used to get the MSP IDs for a channel.
type MSPIDsGetter func(string) []string

// MSPManagerGetter used to get the MSP Manager for a channel.
type MSPManagerGetter func(string) msp.MSPManager

// ---------- the LSCC -----------------

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
	return m, m != nil
}

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
	if cd == nil {
		return errors.New("proto: Marshal called with nil")
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
