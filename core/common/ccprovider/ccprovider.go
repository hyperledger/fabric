/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

var ccproviderLogger = flogging.MustGetLogger("ccprovider")

var chaincodeInstallPath string

// CCPackage encapsulates a chaincode package which can be
//    raw ChaincodeDeploymentSpec
//    SignedChaincodeDeploymentSpec
// Attempt to keep the interface at a level with minimal
// interface for possible generalization.
type CCPackage interface {
	//InitFromBuffer initialize the package from bytes
	InitFromBuffer(buf []byte) (*ChaincodeData, error)

	// InitFromFS gets the chaincode from the filesystem (includes the raw bytes too)
	InitFromFS(ccname string, ccversion string) ([]byte, *pb.ChaincodeDeploymentSpec, error)

	// PutChaincodeToFS writes the chaincode to the filesystem
	PutChaincodeToFS() error

	// GetDepSpec gets the ChaincodeDeploymentSpec from the package
	GetDepSpec() *pb.ChaincodeDeploymentSpec

	// GetDepSpecBytes gets the serialized ChaincodeDeploymentSpec from the package
	GetDepSpecBytes() []byte

	// ValidateCC validates and returns the chaincode deployment spec corresponding to
	// ChaincodeData. The validation is based on the metadata from ChaincodeData
	// One use of this method is to validate the chaincode before launching
	ValidateCC(ccdata *ChaincodeData) error

	// GetPackageObject gets the object as a proto.Message
	GetPackageObject() proto.Message

	// GetChaincodeData gets the ChaincodeData
	GetChaincodeData() *ChaincodeData

	// GetId gets the fingerprint of the chaincode based on package computation
	GetId() []byte
}

// SetChaincodesPath sets the chaincode path for this peer
func SetChaincodesPath(path string) {
	if s, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(path, 0755); err != nil {
				panic(fmt.Sprintf("Could not create chaincodes install path: %s", err))
			}
		} else {
			panic(fmt.Sprintf("Could not stat chaincodes install path: %s", err))
		}
	} else if !s.IsDir() {
		panic(fmt.Errorf("chaincode path exists but not a dir: %s", path))
	}

	chaincodeInstallPath = path
}

func GetChaincodePackage(ccname string, ccversion string) ([]byte, error) {
	return GetChaincodePackageFromPath(ccname, ccversion, chaincodeInstallPath)
}

// isPrintable is used by CDSPackage and SignedCDSPackage validation to
// detect garbage strings in unmarshaled proto fields where printable
// characters are expected.
func isPrintable(name string) bool {
	notASCII := func(r rune) bool {
		return !unicode.IsPrint(r)
	}
	return strings.IndexFunc(name, notASCII) == -1
}

// GetChaincodePackage returns the chaincode package from the file system
func GetChaincodePackageFromPath(ccname string, ccversion string, ccInstallPath string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s.%s", ccInstallPath, ccname, ccversion)
	var ccbytes []byte
	var err error
	if ccbytes, err = ioutil.ReadFile(path); err != nil {
		return nil, err
	}
	return ccbytes, nil
}

// ChaincodePackageExists returns whether the chaincode package exists in the file system
func ChaincodePackageExists(ccname string, ccversion string) (bool, error) {
	path := filepath.Join(chaincodeInstallPath, ccname+"."+ccversion)
	_, err := os.Stat(path)
	if err == nil {
		// chaincodepackage already exists
		return true, nil
	}
	return false, err
}

type CCCacheSupport interface {
	// GetChaincode is needed by the cache to get chaincode data
	GetChaincode(ccname string, ccversion string) (CCPackage, error)
}

// CCInfoFSImpl provides the implementation for CC on the FS and the access to it
// It implements CCCacheSupport
type CCInfoFSImpl struct{}

// GetChaincodeFromFS this is a wrapper for hiding package implementation.
// It calls GetChaincodeFromPath with the chaincodeInstallPath
func (cifs *CCInfoFSImpl) GetChaincode(ccname string, ccversion string) (CCPackage, error) {
	return cifs.GetChaincodeFromPath(ccname, ccversion, chaincodeInstallPath)
}

func (cifs *CCInfoFSImpl) GetChaincodeCodePackage(ccname, ccversion string) ([]byte, error) {
	ccpack, err := cifs.GetChaincode(ccname, ccversion)
	if err != nil {
		return nil, err
	}
	return ccpack.GetDepSpec().Bytes(), nil
}

// GetChaincodeFromPath this is a wrapper for hiding package implementation.
func (*CCInfoFSImpl) GetChaincodeFromPath(ccname string, ccversion string, path string) (CCPackage, error) {
	// try raw CDS
	cccdspack := &CDSPackage{}
	_, _, err := cccdspack.InitFromPath(ccname, ccversion, path)
	if err != nil {
		// try signed CDS
		ccscdspack := &SignedCDSPackage{}
		_, _, err = ccscdspack.InitFromPath(ccname, ccversion, path)
		if err != nil {
			return nil, err
		}
		return ccscdspack, nil
	}
	return cccdspack, nil
}

// PutChaincodeIntoFS is a wrapper for putting raw ChaincodeDeploymentSpec
//using CDSPackage. This is only used in UTs
func (*CCInfoFSImpl) PutChaincode(depSpec *pb.ChaincodeDeploymentSpec) (CCPackage, error) {
	buf, err := proto.Marshal(depSpec)
	if err != nil {
		return nil, err
	}
	cccdspack := &CDSPackage{}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		return nil, err
	}
	err = cccdspack.PutChaincodeToFS()
	if err != nil {
		return nil, err
	}

	return cccdspack, nil
}

// DirEnumerator enumerates directories
type DirEnumerator func(string) ([]os.FileInfo, error)

// ChaincodeExtractor extracts chaincode from a given path
type ChaincodeExtractor func(ccname string, ccversion string, path string) (CCPackage, error)

// ListInstalledChaincodes retrieves the installed chaincodes
func (cifs *CCInfoFSImpl) ListInstalledChaincodes(dir string, ls DirEnumerator, ccFromPath ChaincodeExtractor) ([]chaincode.InstalledChaincode, error) {
	var chaincodes []chaincode.InstalledChaincode
	if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	files, err := ls(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading directory %s", dir)
	}

	for _, f := range files {
		// Skip directories, we're only interested in normal files
		if f.IsDir() {
			continue
		}
		// A chaincode file name is of the type "name.version"
		// We're only interested in the name.
		// Skip files that don't adhere to the file naming convention of "A.B"
		i := strings.Index(f.Name(), ".")
		if i == -1 {
			ccproviderLogger.Info("Skipping", f.Name(), "because of missing separator '.'")
			continue
		}
		ccName := f.Name()[:i]      // Everything before the separator
		ccVersion := f.Name()[i+1:] // Everything after the separator

		ccPackage, err := ccFromPath(ccName, ccVersion, dir)
		if err != nil {
			ccproviderLogger.Warning("Failed obtaining chaincode information about", ccName, ccVersion, ":", err)
			return nil, errors.Wrapf(err, "failed obtaining information about %s, version %s", ccName, ccVersion)
		}

		chaincodes = append(chaincodes, chaincode.InstalledChaincode{
			Name:    ccName,
			Version: ccVersion,
			Id:      ccPackage.GetId(),
		})
	}
	ccproviderLogger.Debug("Returning", chaincodes)
	return chaincodes, nil
}

// ccInfoFSStorageMgr is the storage manager used either by the cache or if the
// cache is bypassed
var ccInfoFSProvider = &CCInfoFSImpl{}

// ccInfoCache is the cache instance itself
var ccInfoCache = NewCCInfoCache(ccInfoFSProvider)

// GetChaincodeFromFS retrieves chaincode information from the file system
func GetChaincodeFromFS(ccname string, ccversion string) (CCPackage, error) {
	return ccInfoFSProvider.GetChaincode(ccname, ccversion)
}

// PutChaincodeIntoFS puts chaincode information in the file system (and
// also in the cache to prime it) if the cache is enabled, or directly
// from the file system otherwise
func PutChaincodeIntoFS(depSpec *pb.ChaincodeDeploymentSpec) error {
	_, err := ccInfoFSProvider.PutChaincode(depSpec)
	return err
}

// GetChaincodeData gets chaincode data from cache if there's one
func GetChaincodeData(ccname string, ccversion string) (*ChaincodeData, error) {
	ccproviderLogger.Debugf("Getting chaincode data for <%s, %s> from cache", ccname, ccversion)
	return ccInfoCache.GetChaincodeData(ccname, ccversion)
}

func CheckInstantiationPolicy(name, version string, cdLedger *ChaincodeData) error {
	ccdata, err := GetChaincodeData(name, version)
	if err != nil {
		return err
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
	if ccdata.InstantiationPolicy != nil {
		if !bytes.Equal(ccdata.InstantiationPolicy, cdLedger.InstantiationPolicy) {
			return fmt.Errorf("Instantiation policy mismatch for cc %s/%s", name, version)
		}
	}

	return nil
}

// GetCCPackage tries each known package implementation one by one
// till the right package is found
func GetCCPackage(buf []byte) (CCPackage, error) {
	// try raw CDS
	cds := &CDSPackage{}
	if ccdata, err := cds.InitFromBuffer(buf); err != nil {
		cds = nil
	} else {
		err = cds.ValidateCC(ccdata)
		if err != nil {
			cds = nil
		}
	}

	// try signed CDS
	scds := &SignedCDSPackage{}
	if ccdata, err := scds.InitFromBuffer(buf); err != nil {
		scds = nil
	} else {
		err = scds.ValidateCC(ccdata)
		if err != nil {
			scds = nil
		}
	}

	if cds != nil && scds != nil {
		// Both were unmarshaled successfully, this is exactly why the approach of
		// hoping proto fails for bad inputs is fatally flawed.
		ccproviderLogger.Errorf("Could not determine chaincode package type, guessing SignedCDS")
		return scds, nil
	}

	if cds != nil {
		return cds, nil
	}

	if scds != nil {
		return scds, nil
	}

	return nil, errors.New("could not unmarshal chaincode package to CDS or SignedCDS")
}

// GetInstalledChaincodes returns a map whose key is the chaincode id and
// value is the ChaincodeDeploymentSpec struct for that chaincodes that have
// been installed (but not necessarily instantiated) on the peer by searching
// the chaincode install path
func GetInstalledChaincodes() (*pb.ChaincodeQueryResponse, error) {
	files, err := ioutil.ReadDir(chaincodeInstallPath)
	if err != nil {
		return nil, err
	}

	// array to store info for all chaincode entries from LSCC
	var ccInfoArray []*pb.ChaincodeInfo

	for _, file := range files {
		// split at first period as chaincode versions can contain periods while
		// chaincode names cannot
		fileNameArray := strings.SplitN(file.Name(), ".", 2)

		// check that length is 2 as expected, otherwise skip to next cc file
		if len(fileNameArray) == 2 {
			ccname := fileNameArray[0]
			ccversion := fileNameArray[1]
			ccpack, err := GetChaincodeFromFS(ccname, ccversion)
			if err != nil {
				// either chaincode on filesystem has been tampered with or
				// a non-chaincode file has been found in the chaincodes directory
				ccproviderLogger.Errorf("Unreadable chaincode file found on filesystem: %s", file.Name())
				continue
			}

			cdsfs := ccpack.GetDepSpec()

			name := cdsfs.GetChaincodeSpec().GetChaincodeId().Name
			version := cdsfs.GetChaincodeSpec().GetChaincodeId().Version
			if name != ccname || version != ccversion {
				// chaincode name/version in the chaincode file name has been modified
				// by an external entity
				ccproviderLogger.Errorf("Chaincode file's name/version has been modified on the filesystem: %s", file.Name())
				continue
			}

			path := cdsfs.GetChaincodeSpec().ChaincodeId.Path
			// since this is just an installed chaincode these should be blank
			input, escc, vscc := "", "", ""

			ccInfo := &pb.ChaincodeInfo{Name: name, Version: version, Path: path, Input: input, Escc: escc, Vscc: vscc, Id: ccpack.GetId()}

			// add this specific chaincode's metadata to the array of all chaincodes
			ccInfoArray = append(ccInfoArray, ccInfo)
		}
	}
	// add array with info about all instantiated chaincodes to the query
	// response proto
	cqr := &pb.ChaincodeQueryResponse{Chaincodes: ccInfoArray}

	return cqr, nil
}

// CCContext pass this around instead of string of args
type CCContext struct {
	// Name chaincode name
	Name string

	// Version used to construct the chaincode image and register
	Version string
}

// GetCanonicalName returns the canonical name associated with the proposal context
func (cccid *CCContext) GetCanonicalName() string {
	return cccid.Name + ":" + cccid.Version
}

//-------- ChaincodeDefinition - interface for ChaincodeData ------
// ChaincodeDefinition describes all of the necessary information for a peer to decide whether to endorse
// a proposal and whether to validate a transaction, for a particular chaincode.
type ChaincodeDefinition interface {
	// CCName returns the name of this chaincode (the name it was put in the ChaincodeRegistry with).
	CCName() string

	// Hash returns the hash of the chaincode.
	Hash() []byte

	// CCVersion returns the version of the chaincode.
	CCVersion() string

	// Validation returns how to validate transactions for this chaincode.
	// The string returned is the name of the validation method (usually 'vscc')
	// and the bytes returned are the argument to the validation (in the case of
	// 'vscc', this is a marshaled pb.VSCCArgs message).
	Validation() (string, []byte)

	// Endorsement returns how to endorse proposals for this chaincode.
	// The string returns is the name of the endorsement method (usually 'escc').
	Endorsement() string
}

//-------- ChaincodeData is stored on the LSCC -------

// ChaincodeData defines the datastructure for chaincodes to be serialized by proto
// Type provides an additional check by directing to use a specific package after instantiation
// Data is Type specific (see CDSPackage and SignedCDSPackage)
type ChaincodeData struct {
	// Name of the chaincode
	Name string `protobuf:"bytes,1,opt,name=name"`

	// Version of the chaincode
	Version string `protobuf:"bytes,2,opt,name=version"`

	// Escc for the chaincode instance
	Escc string `protobuf:"bytes,3,opt,name=escc"`

	// Vscc for the chaincode instance
	Vscc string `protobuf:"bytes,4,opt,name=vscc"`

	// Policy endorsement policy for the chaincode instance
	Policy []byte `protobuf:"bytes,5,opt,name=policy,proto3"`

	// Data data specific to the package
	Data []byte `protobuf:"bytes,6,opt,name=data,proto3"`

	// Id of the chaincode that's the unique fingerprint for the CC This is not
	// currently used anywhere but serves as a good eyecatcher
	Id []byte `protobuf:"bytes,7,opt,name=id,proto3"`

	// InstantiationPolicy for the chaincode
	InstantiationPolicy []byte `protobuf:"bytes,8,opt,name=instantiation_policy,proto3"`
}

// CCName returns the name of this chaincode (the name it was put in the ChaincodeRegistry with).
func (cd *ChaincodeData) CCName() string {
	return cd.Name
}

// Hash returns the hash of the chaincode.
func (cd *ChaincodeData) Hash() []byte {
	return cd.Id
}

// CCVersion returns the version of the chaincode.
func (cd *ChaincodeData) CCVersion() string {
	return cd.Version
}

// Validation returns how to validate transactions for this chaincode.
// The string returned is the name of the validation method (usually 'vscc')
// and the bytes returned are the argument to the validation (in the case of
// 'vscc', this is a marshaled pb.VSCCArgs message).
func (cd *ChaincodeData) Validation() (string, []byte) {
	return cd.Vscc, cd.Policy
}

// Endorsement returns how to endorse proposals for this chaincode.
// The string returns is the name of the endorsement method (usually 'escc').
func (cd *ChaincodeData) Endorsement() string {
	return cd.Escc
}

// implement functions needed from proto.Message for proto's mar/unmarshal functions

// Reset resets
func (cd *ChaincodeData) Reset() { *cd = ChaincodeData{} }

// String converts to string
func (cd *ChaincodeData) String() string { return proto.CompactTextString(cd) }

// ProtoMessage just exists to make proto happy
func (*ChaincodeData) ProtoMessage() {}

// ChaincodeContainerInfo is yet another synonym for the data required to start/stop a chaincode.
type ChaincodeContainerInfo struct {
	Name        string
	Version     string
	Path        string
	Type        string
	CodePackage []byte

	// ContainerType is not a great name, but 'DOCKER' and 'SYSTEM' are the valid types
	ContainerType string
}

// TransactionParams are parameters which are tied to a particular transaction
// and which are required for invoking chaincode.
type TransactionParams struct {
	TxID                 string
	ChannelID            string
	SignedProp           *pb.SignedProposal
	Proposal             *pb.Proposal
	TXSimulator          ledger.TxSimulator
	HistoryQueryExecutor ledger.HistoryQueryExecutor
	CollectionStore      privdata.CollectionStore
	IsInitTransaction    bool

	// this is additional data passed to the chaincode
	ProposalDecorations map[string][]byte
}

// ChaincodeProvider provides an abstraction layer that is
// used for different packages to interact with code in the
// chaincode package without importing it; more methods
// should be added below if necessary
type ChaincodeProvider interface {
	// Execute executes a standard chaincode invocation for a chaincode and an input
	Execute(txParams *TransactionParams, cccid *CCContext, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error)
	// ExecuteLegacyInit is a special case for executing chaincode deployment specs,
	// which are not already in the LSCC, needed for old lifecycle
	ExecuteLegacyInit(txParams *TransactionParams, cccid *CCContext, spec *pb.ChaincodeDeploymentSpec) (*pb.Response, *pb.ChaincodeEvent, error)
	// Stop stops the chaincode give
	Stop(ccci *ChaincodeContainerInfo) error
}

func DeploymentSpecToChaincodeContainerInfo(cds *pb.ChaincodeDeploymentSpec) *ChaincodeContainerInfo {
	return &ChaincodeContainerInfo{
		Name:          cds.Name(),
		Version:       cds.Version(),
		Path:          cds.Path(),
		Type:          cds.CCType(),
		ContainerType: cds.ExecEnv.String(),
	}
}
