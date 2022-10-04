/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
)

var ccproviderLogger = flogging.MustGetLogger("ccprovider")

var chaincodeInstallPath string

// CCPackage encapsulates a chaincode package which can be
//
//	raw ChaincodeDeploymentSpec
//	SignedChaincodeDeploymentSpec
//
// Attempt to keep the interface at a level with minimal
// interface for possible generalization.
type CCPackage interface {
	// InitFromBuffer initialize the package from bytes
	InitFromBuffer(buf []byte) (*ChaincodeData, error)

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
			if err := os.Mkdir(path, 0o755); err != nil {
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
func GetChaincodePackageFromPath(ccNameVersion string, ccInstallPath string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s", ccInstallPath, strings.ReplaceAll(ccNameVersion, ":", "."))
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
	GetChaincode(ccNameVersion string) (CCPackage, error)
}

// CCInfoFSImpl provides the implementation for CC on the FS and the access to it
// It implements CCCacheSupport
type CCInfoFSImpl struct {
	GetHasher GetHasher
}

// GetChaincodeFromFS this is a wrapper for hiding package implementation.
// It calls GetChaincodeFromPath with the chaincodeInstallPath
func (cifs *CCInfoFSImpl) GetChaincode(ccNameVersion string) (CCPackage, error) {
	return cifs.GetChaincodeFromPath(ccNameVersion, chaincodeInstallPath)
}

func (cifs *CCInfoFSImpl) GetChaincodeCodePackage(ccNameVersion string) ([]byte, error) {
	ccpack, err := cifs.GetChaincode(ccNameVersion)
	if err != nil {
		return nil, err
	}
	return ccpack.GetDepSpec().CodePackage, nil
}

func (cifs *CCInfoFSImpl) GetChaincodeDepSpec(ccNameVersion string) (*pb.ChaincodeDeploymentSpec, error) {
	ccpack, err := cifs.GetChaincode(ccNameVersion)
	if err != nil {
		return nil, err
	}
	return ccpack.GetDepSpec(), nil
}

// GetChaincodeFromPath this is a wrapper for hiding package implementation.
func (cifs *CCInfoFSImpl) GetChaincodeFromPath(ccNameVersion string, path string) (CCPackage, error) {
	// try raw CDS
	cccdspack := &CDSPackage{GetHasher: cifs.GetHasher}
	_, _, err := cccdspack.InitFromPath(ccNameVersion, path)
	if err != nil {
		// try signed CDS
		ccscdspack := &SignedCDSPackage{GetHasher: cifs.GetHasher}
		_, _, err = ccscdspack.InitFromPath(ccNameVersion, path)
		if err != nil {
			return nil, err
		}
		return ccscdspack, nil
	}
	return cccdspack, nil
}

// GetChaincodeInstallPath returns the path to the installed chaincodes
func (*CCInfoFSImpl) GetChaincodeInstallPath() string {
	return chaincodeInstallPath
}

// PutChaincode is a wrapper for putting raw ChaincodeDeploymentSpec
// using CDSPackage. This is only used in UTs
func (cifs *CCInfoFSImpl) PutChaincode(depSpec *pb.ChaincodeDeploymentSpec) (CCPackage, error) {
	buf, err := proto.Marshal(depSpec)
	if err != nil {
		return nil, err
	}
	cccdspack := &CDSPackage{GetHasher: cifs.GetHasher}
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
type ChaincodeExtractor func(ccNameVersion string, path string, getHasher GetHasher) (CCPackage, error)

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

		ccPackage, err := ccFromPath(ccName+":"+ccVersion, dir, cifs.GetHasher)
		if err != nil {
			ccproviderLogger.Warning("Failed obtaining chaincode information about", ccName, ccVersion, ":", err)
			return nil, errors.Wrapf(err, "failed obtaining information about %s, version %s", ccName, ccVersion)
		}

		chaincodes = append(chaincodes, chaincode.InstalledChaincode{
			Name:    ccName,
			Version: ccVersion,
			Hash:    ccPackage.GetId(),
		})
	}
	ccproviderLogger.Debug("Returning", chaincodes)
	return chaincodes, nil
}

// ccInfoFSStorageMgr is the storage manager used either by the cache or if the
// cache is bypassed
var ccInfoFSProvider = &CCInfoFSImpl{GetHasher: factory.GetDefault()}

// ccInfoCache is the cache instance itself
var ccInfoCache = NewCCInfoCache(ccInfoFSProvider)

// GetChaincodeFromFS retrieves chaincode information from the file system
func GetChaincodeFromFS(ccNameVersion string) (CCPackage, error) {
	return ccInfoFSProvider.GetChaincode(ccNameVersion)
}

// GetChaincodeData gets chaincode data from cache if there's one
func GetChaincodeData(ccNameVersion string) (*ChaincodeData, error) {
	ccproviderLogger.Debugf("Getting chaincode data for <%s> from cache", ccNameVersion)
	return ccInfoCache.GetChaincodeData(ccNameVersion)
}

// GetCCPackage tries each known package implementation one by one
// till the right package is found
func GetCCPackage(buf []byte, bccsp bccsp.BCCSP) (CCPackage, error) {
	// try raw CDS
	cds := &CDSPackage{GetHasher: bccsp}
	if ccdata, err := cds.InitFromBuffer(buf); err != nil {
		cds = nil
	} else {
		err = cds.ValidateCC(ccdata)
		if err != nil {
			cds = nil
		}
	}

	// try signed CDS
	scds := &SignedCDSPackage{GetHasher: bccsp}
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
			ccpack, err := GetChaincodeFromFS(ccname + ":" + ccversion)
			if err != nil {
				// either chaincode on filesystem has been tampered with or
				// _lifecycle chaincode files exist in the chaincodes directory.
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

// ChaincodeID is the name by which the chaincode will register itself.
func (cd *ChaincodeData) ChaincodeID() string {
	return cd.Name + ":" + cd.Version
}

// implement functions needed from proto.Message for proto's mar/unmarshal functions

// Reset resets
func (cd *ChaincodeData) Reset() { *cd = ChaincodeData{} }

// String converts to string
func (cd *ChaincodeData) String() string { return proto.CompactTextString(cd) }

// ProtoMessage just exists to make proto happy
func (*ChaincodeData) ProtoMessage() {}

// TransactionParams are parameters which are tied to a particular transaction
// and which are required for invoking chaincode.
type TransactionParams struct {
	TxID                 string
	ChannelID            string
	NamespaceID          string
	SignedProp           *pb.SignedProposal
	Proposal             *pb.Proposal
	TXSimulator          ledger.TxSimulator
	HistoryQueryExecutor ledger.HistoryQueryExecutor
	CollectionStore      privdata.CollectionStore
	IsInitTransaction    bool

	// this is additional data passed to the chaincode
	ProposalDecorations map[string][]byte
}
