// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"github.com/hyperledger/fabric/internal/fileutil"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/pkg/errors"
)

const filePermPrivateRW os.FileMode = 0600

type fileBootstrapper struct {
	GenesisBlockFile string
}

// New returns a new static bootstrap helper.
func New(fileName string) bootstrap.Helper {
	return &fileBootstrapper{
		GenesisBlockFile: fileName,
	}
}

// GenesisBlock returns the genesis block to be used for bootstrapping.
func (b *fileBootstrapper) GenesisBlock() *cb.Block {
	bootstrapFile, fileErr := ioutil.ReadFile(b.GenesisBlockFile)
	if fileErr != nil {
		panic(errors.Errorf("unable to bootstrap orderer. Error reading genesis block file: %v", fileErr))
	}
	genesisBlock := &cb.Block{}
	unmarshallErr := proto.Unmarshal(bootstrapFile, genesisBlock)
	if unmarshallErr != nil {
		panic(errors.Errorf("unable to bootstrap orderer. Error unmarshalling genesis block: %v", unmarshallErr))

	}
	return genesisBlock
} // GenesisBlock

func (b *fileBootstrapper) SaveBlock(block *cb.Block) error {
	blockBytes, err := proto.Marshal(block)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal block")
	}
	err = fileutil.CreateAndSyncFile(b.GenesisBlockFile, blockBytes, filePermPrivateRW)
	if err != nil {
		return err
	}

	return nil
}
