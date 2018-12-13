// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

type fileBootstrapper struct {
	GenesisBlockFile string
}

// New returns a new static bootstrap helper.
func New(fileName string) bootstrap.Helper {
	return &fileBootstrapper{
		GenesisBlockFile: fileName,
	}
}

// NewReplacer returns a new bootstrap replacer.
func NewReplacer(fileName string) bootstrap.Replacer {
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

// ReplaceGenesisBlockFile creates a backup of the genesis block file, and then replaces
// it with the content of the given block.
// This is used during consensus-type migration in order to generate a bootstrap file that
// specifies the new consensus-type.
func (b *fileBootstrapper) ReplaceGenesisBlockFile(block *cb.Block) error {
	buff, marshalErr := proto.Marshal(block)
	if marshalErr != nil {
		return errors.Wrap(marshalErr, "could not marshal block into a []byte")
	}

	genFileStat, statErr := os.Stat(b.GenesisBlockFile)
	if statErr != nil {
		return errors.Wrapf(statErr, "could not get the os.Stat of the genesis block file: %s", b.GenesisBlockFile)
	}

	if !genFileStat.Mode().IsRegular() {
		return errors.Errorf("genesis block file: %s, is not a regular file", b.GenesisBlockFile)
	}

	backupFile := b.GenesisBlockFile + ".bak"
	if err := backupGenesisFile(b.GenesisBlockFile, backupFile); err != nil {
		return errors.Wrapf(err, "could not copy genesis block file (%s) into backup file: %s",
			b.GenesisBlockFile, backupFile)
	}

	if err := ioutil.WriteFile(b.GenesisBlockFile, buff, genFileStat.Mode()); err != nil {
		return errors.Wrapf(err, "could not write new genesis block into file: %s; use backup if necessary: %s",
			b.GenesisBlockFile, backupFile)
	}

	return nil
}

func backupGenesisFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func (b *fileBootstrapper) CheckReadWrite() error {
	genFileStat, statErr := os.Stat(b.GenesisBlockFile)
	if statErr != nil {
		return errors.Wrapf(statErr, "could not get the os.Stat of the genesis block file: %s", b.GenesisBlockFile)
	}

	if !genFileStat.Mode().IsRegular() {
		return errors.Errorf("genesis block file: %s, is not a regular file", b.GenesisBlockFile)
	}

	genFile, openErr := os.OpenFile(b.GenesisBlockFile, os.O_RDWR, genFileStat.Mode().Perm())
	if openErr != nil {
		if os.IsPermission(openErr) {
			return errors.Wrapf(openErr, "genesis block file: %s, cannot be opened for read-write, check permissions", b.GenesisBlockFile)
		} else {
			return errors.Wrapf(openErr, "genesis block file: %s, cannot be opened for read-write", b.GenesisBlockFile)
		}
	}
	genFile.Close()

	return nil
}
