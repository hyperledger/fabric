/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	protopeer "github.com/hyperledger/fabric/protos/peer"
	prototestutils "github.com/hyperledger/fabric/protos/testutils"
	"github.com/hyperledger/fabric/protos/utils"
)

var logger = flogging.MustGetLogger("test2")

// collConf helps writing tests with less verbose code by specifying coll configuration
// in a simple struct in place of 'common.CollectionConfigPackage'. (the test heplers' apis
// use 'collConf' as parameters and return values and transform back and forth to/from proto
// message internally (using func 'convertToCollConfigProtoBytes' and 'convertFromCollConfigProto')
type collConf struct {
	name string
	btl  uint64
}

type txAndPvtdata struct {
	Txid     string
	Envelope *common.Envelope
	Pvtws    *rwset.TxPvtReadWriteSet
}

func convertToCollConfigProtoBytes(collConfs []*collConf) ([]byte, error) {
	var protoConfArray []*common.CollectionConfig
	for _, c := range collConfs {
		protoConf := &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &common.StaticCollectionConfig{Name: c.name, BlockToLive: c.btl}}}
		protoConfArray = append(protoConfArray, protoConf)
	}
	return proto.Marshal(&common.CollectionConfigPackage{Config: protoConfArray})
}

func convertFromCollConfigProto(collConfPkg *common.CollectionConfigPackage) []*collConf {
	var collConfs []*collConf
	protoConfArray := collConfPkg.Config
	for _, protoConf := range protoConfArray {
		name, btl := protoConf.GetStaticCollectionConfig().Name, protoConf.GetStaticCollectionConfig().BlockToLive
		collConfs = append(collConfs, &collConf{name, btl})
	}
	return collConfs
}

func constructTransaction(txid string, simulationResults []byte) (*common.Envelope, error) {
	channelid := "dummyChannel"
	ccid := &protopeer.ChaincodeID{
		Name:    "dummyCC",
		Version: "dummyVer",
	}
	txenv, _, err := prototestutils.ConstructUnsignedTxEnv(channelid, ccid, &protopeer.Response{Status: 200}, simulationResults, txid, nil, nil)
	return txenv, err
}

func constructTestGenesisBlock(channelid string) (*common.Block, error) {
	blk, err := configtxtest.MakeGenesisBlock(channelid)
	if err != nil {
		return nil, err
	}
	setBlockFlagsToValid(blk)
	return blk, nil
}

func setBlockFlagsToValid(block *common.Block) {
	utils.InitBlockMetadata(block)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] =
		lutils.NewTxValidationFlagsSetValue(len(block.Data.Data), protopeer.TxValidationCode_VALID)
}

// createZipFile will zip a source directory or file into targetZipFile.
func createZipFile(sourcePath, targetZipFile string) error {
	zipfile, err := os.Create(targetZipFile)
	if err != nil {
		return err
	}
	defer zipfile.Close()
	zipWriter := zip.NewWriter(zipfile)
	defer zipWriter.Close()

	baseDirName := ""
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil
	}
	if fileInfo.IsDir() {
		baseDirName = filepath.Base(sourcePath)
	}

	filepath.Walk(sourcePath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(fileInfo)
		if err != nil {
			return err
		}

		if baseDirName != "" {
			header.Name = filepath.Join(baseDirName, strings.TrimPrefix(path, sourcePath))
		}
		if fileInfo.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if fileInfo.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})

	return err
}
