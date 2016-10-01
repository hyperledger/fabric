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

package fsblkstorage

import (
	"fmt"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledgernext/util"
	"github.com/hyperledger/fabric/core/ledgernext/util/db"
	"github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

var logger = logging.MustGetLogger("kvledger")

const (
	blockIndexCF    = "blockIndexCF"
	blockfilePrefix = "blockfile_"
)

var (
	blkMgrInfoKey = []byte("blkMgrInfo")
)

type blockfileMgr struct {
	rootDir           string
	conf              *Conf
	db                *db.DB
	defaultCF         *gorocksdb.ColumnFamilyHandle
	index             index
	cpInfo            *checkpointInfo
	currentFileWriter *blockfileWriter
	bcInfo            atomic.Value
}

func newBlockfileMgr(conf *Conf) *blockfileMgr {
	rootDir := conf.blockfilesDir
	_, err := util.CreateDirIfMissing(rootDir)
	if err != nil {
		panic(fmt.Sprintf("Error: %s", err))
	}
	db := initDB(conf)
	mgr := &blockfileMgr{rootDir: rootDir, conf: conf, db: db, defaultCF: db.GetDefaultCFHandle()}
	cpInfo, err := mgr.loadCurrentInfo()
	if err != nil {
		panic(fmt.Sprintf("Could not get block file info for current block file from db: %s", err))
	}
	if cpInfo == nil {
		cpInfo = &checkpointInfo{latestFileChunkSuffixNum: 0, latestFileChunksize: 0}
		err = mgr.saveCurrentInfo(cpInfo, true)
		if err != nil {
			panic(fmt.Sprintf("Could not save next block file info to db: %s", err))
		}
	}
	updateCPInfo(conf, cpInfo)
	currentFileWriter, err := newBlockfileWriter(deriveBlockfilePath(rootDir, cpInfo.latestFileChunkSuffixNum))
	if err != nil {
		panic(fmt.Sprintf("Could not open writer to current file: %s", err))
	}
	err = currentFileWriter.truncateFile(cpInfo.latestFileChunksize)
	if err != nil {
		panic(fmt.Sprintf("Could not truncate current file to known size in db: %s", err))
	}

	mgr.index = newBlockIndex(db, db.GetCFHandle(blockIndexCF))
	mgr.cpInfo = cpInfo
	mgr.currentFileWriter = currentFileWriter
	mgr.syncIndex()

	// init BlockchainInfo
	bcInfo := &protos.BlockchainInfo{
		Height:            0,
		CurrentBlockHash:  nil,
		PreviousBlockHash: nil}

	if cpInfo.lastBlockNumber > 0 {
		lastBlock, err := mgr.retrieveSerBlockByNumber(cpInfo.lastBlockNumber)
		if err != nil {
			panic(fmt.Sprintf("Could not retrieve last block form file: %s", err))
		}
		lastBlockHash := lastBlock.ComputeHash()
		previousBlockHash, err := lastBlock.GetPreviousBlockHash()
		if err != nil {
			panic(fmt.Sprintf("Error in decoding block: %s", err))
		}
		bcInfo = &protos.BlockchainInfo{
			Height:            cpInfo.lastBlockNumber,
			CurrentBlockHash:  lastBlockHash,
			PreviousBlockHash: previousBlockHash}
	}
	mgr.bcInfo.Store(bcInfo)
	return mgr
}

func initDB(conf *Conf) *db.DB {
	dbInst := db.CreateDB(&db.Conf{
		DBPath:     conf.dbPath,
		CFNames:    []string{blockIndexCF},
		DisableWAL: true})

	dbInst.Open()
	return dbInst
}

func updateCPInfo(conf *Conf, cpInfo *checkpointInfo) {
	logger.Debugf("Starting checkpoint=%s", cpInfo)
	rootDir := conf.blockfilesDir
	filePath := deriveBlockfilePath(rootDir, cpInfo.latestFileChunkSuffixNum)
	exists, size, err := util.FileExists(filePath)
	if err != nil {
		panic(fmt.Sprintf("Error in checking whether file [%s] exists: %s", filePath, err))
	}
	logger.Debugf("status of file [%s]: exists=[%t], size=[%d]", filePath, exists, size)
	if !exists || int(size) == cpInfo.latestFileChunksize {
		// check point info is in sync with the file on disk
		return
	}
	endOffsetLastBlock, numBlocks, err := scanForLastCompleteBlock(
		rootDir, cpInfo.latestFileChunkSuffixNum, int64(cpInfo.latestFileChunksize))
	if err != nil {
		panic(fmt.Sprintf("Could not open current file for detecting last block in the file: %s", err))
	}
	cpInfo.lastBlockNumber += uint64(numBlocks)
	cpInfo.latestFileChunksize = int(endOffsetLastBlock)
	logger.Debugf("Checkpoint after updates by scanning the last file segment:%s", cpInfo)
}

func deriveBlockfilePath(rootDir string, suffixNum int) string {
	return rootDir + "/" + blockfilePrefix + fmt.Sprintf("%06d", suffixNum)
}

func (mgr *blockfileMgr) open() error {
	return mgr.currentFileWriter.open()
}

func (mgr *blockfileMgr) close() {
	mgr.currentFileWriter.close()
	mgr.db.Close()
}

func (mgr *blockfileMgr) moveToNextFile() {
	nextFileInfo := &checkpointInfo{
		latestFileChunkSuffixNum: mgr.cpInfo.latestFileChunkSuffixNum + 1,
		latestFileChunksize:      0}

	nextFileWriter, err := newBlockfileWriter(
		deriveBlockfilePath(mgr.rootDir, nextFileInfo.latestFileChunkSuffixNum))

	if err != nil {
		panic(fmt.Sprintf("Could not open writer to next file: %s", err))
	}
	mgr.currentFileWriter.close()
	err = mgr.saveCurrentInfo(nextFileInfo, true)
	if err != nil {
		panic(fmt.Sprintf("Could not save next block file info to db: %s", err))
	}
	mgr.cpInfo = nextFileInfo
	mgr.currentFileWriter = nextFileWriter
}

func (mgr *blockfileMgr) addBlock(block *protos.Block2) error {
	serBlock, err := protos.ConstructSerBlock2(block)
	if err != nil {
		return fmt.Errorf("Error while serializing block: %s", err)
	}
	blockBytes := serBlock.GetBytes()
	blockHash := serBlock.ComputeHash()
	txOffsets, err := serBlock.GetTxOffsets()
	currentOffset := mgr.cpInfo.latestFileChunksize
	if err != nil {
		return fmt.Errorf("Error while serializing block: %s", err)
	}
	blockBytesLen := len(blockBytes)
	blockBytesEncodedLen := proto.EncodeVarint(uint64(blockBytesLen))
	totalBytesToAppend := blockBytesLen + len(blockBytesEncodedLen)

	if currentOffset+totalBytesToAppend > mgr.conf.maxBlockfileSize {
		mgr.moveToNextFile()
		currentOffset = 0
	}
	err = mgr.currentFileWriter.append(blockBytesEncodedLen, false)
	if err == nil {
		err = mgr.currentFileWriter.append(blockBytes, true)
	}
	if err != nil {
		truncateErr := mgr.currentFileWriter.truncateFile(mgr.cpInfo.latestFileChunksize)
		if truncateErr != nil {
			panic(fmt.Sprintf("Could not truncate current file to known size after an error during block append: %s", err))
		}
		return fmt.Errorf("Error while appending block to file: %s", err)
	}

	mgr.cpInfo.latestFileChunksize += totalBytesToAppend
	mgr.cpInfo.lastBlockNumber++
	err = mgr.saveCurrentInfo(mgr.cpInfo, false)
	if err != nil {
		mgr.cpInfo.latestFileChunksize -= totalBytesToAppend
		truncateErr := mgr.currentFileWriter.truncateFile(mgr.cpInfo.latestFileChunksize)
		if truncateErr != nil {
			panic(fmt.Sprintf("Error in truncating current file to known size after an error in saving checkpoint info: %s", err))
		}
		return fmt.Errorf("Error while saving current file info to db: %s", err)
	}
	blockFLP := &fileLocPointer{fileSuffixNum: mgr.cpInfo.latestFileChunkSuffixNum}
	blockFLP.offset = currentOffset
	// shift the txoffset because we prepend length of bytes before block bytes
	for i := 0; i < len(txOffsets); i++ {
		txOffsets[i] += len(blockBytesEncodedLen)
	}
	mgr.index.indexBlock(&blockIdxInfo{
		blockNum: mgr.cpInfo.lastBlockNumber, blockHash: blockHash,
		flp: blockFLP, txOffsets: txOffsets})
	mgr.updateBlockchainInfo(blockHash, block)
	return nil
}

func (mgr *blockfileMgr) syncIndex() error {
	var lastBlockIndexed uint64
	var err error
	if lastBlockIndexed, err = mgr.index.getLastBlockIndexed(); err != nil {
		return err
	}
	startFileNum := 0
	startOffset := 0
	blockNum := uint64(1)
	endFileNum := mgr.cpInfo.latestFileChunkSuffixNum
	if lastBlockIndexed != 0 {
		var flp *fileLocPointer
		if flp, err = mgr.index.getBlockLocByBlockNum(lastBlockIndexed); err != nil {
			return err
		}
		startFileNum = flp.fileSuffixNum
		startOffset = flp.locPointer.offset
		blockNum = lastBlockIndexed
	}

	var stream *blockStream
	if stream, err = newBlockStream(mgr.rootDir, startFileNum, int64(startOffset), endFileNum); err != nil {
		return err
	}
	var blockBytes []byte
	var blockPlacementInfo *blockPlacementInfo

	for {
		if blockBytes, blockPlacementInfo, err = stream.nextBlockBytesAndPlacementInfo(); err != nil {
			return err
		}
		if blockBytes == nil {
			break
		}
		serBlock2 := protos.NewSerBlock2(blockBytes)
		var txOffsets []int
		if txOffsets, err = serBlock2.GetTxOffsets(); err != nil {
			return err
		}
		for i := 0; i < len(txOffsets); i++ {
			txOffsets[i] += int(blockPlacementInfo.blockBytesOffset)
		}
		blockIdxInfo := &blockIdxInfo{}
		blockIdxInfo.blockHash = serBlock2.ComputeHash()
		blockIdxInfo.blockNum = blockNum
		blockIdxInfo.flp = &fileLocPointer{fileSuffixNum: blockPlacementInfo.fileNum,
			locPointer: locPointer{offset: int(blockPlacementInfo.blockStartOffset)}}
		blockIdxInfo.txOffsets = txOffsets
		if err = mgr.index.indexBlock(blockIdxInfo); err != nil {
			return err
		}
		blockNum++
	}
	return nil
}

func (mgr *blockfileMgr) getBlockchainInfo() *protos.BlockchainInfo {
	return mgr.bcInfo.Load().(*protos.BlockchainInfo)
}

func (mgr *blockfileMgr) updateBlockchainInfo(latestBlockHash []byte, latestBlock *protos.Block2) {
	currentBCInfo := mgr.getBlockchainInfo()
	newBCInfo := &protos.BlockchainInfo{
		Height:            currentBCInfo.Height + 1,
		CurrentBlockHash:  latestBlockHash,
		PreviousBlockHash: latestBlock.PreviousBlockHash}

	mgr.bcInfo.Store(newBCInfo)
}

func (mgr *blockfileMgr) retrieveBlockByHash(blockHash []byte) (*protos.Block2, error) {
	logger.Debugf("retrieveBlockByHash() - blockHash = [%#v]", blockHash)
	loc, err := mgr.index.getBlockLocByHash(blockHash)
	if err != nil {
		return nil, err
	}
	return mgr.fetchBlock(loc)
}

func (mgr *blockfileMgr) retrieveBlockByNumber(blockNum uint64) (*protos.Block2, error) {
	logger.Debugf("retrieveBlockByNumber() - blockNum = [%d]", blockNum)
	loc, err := mgr.index.getBlockLocByBlockNum(blockNum)
	if err != nil {
		return nil, err
	}
	return mgr.fetchBlock(loc)
}

func (mgr *blockfileMgr) retrieveSerBlockByNumber(blockNum uint64) (*protos.SerBlock2, error) {
	logger.Debugf("retrieveSerBlockByNumber() - blockNum = [%d]", blockNum)
	loc, err := mgr.index.getBlockLocByBlockNum(blockNum)
	if err != nil {
		return nil, err
	}
	return mgr.fetchSerBlock(loc)
}

func (mgr *blockfileMgr) retrieveBlocks(startNum uint64, endNum uint64) (*BlocksItr, error) {
	var lp *fileLocPointer
	var err error
	if lp, err = mgr.index.getBlockLocByBlockNum(startNum); err != nil {
		return nil, err
	}
	var stream *blockStream
	if stream, err = newBlockStream(mgr.rootDir, lp.fileSuffixNum,
		int64(lp.offset), mgr.cpInfo.latestFileChunkSuffixNum); err != nil {
		return nil, err
	}
	return newBlockItr(stream, int(endNum-startNum)+1), nil
}

func (mgr *blockfileMgr) retrieveTransactionByID(txID string) (*protos.Transaction2, error) {
	logger.Debugf("retrieveTransactionByID() - txId = [%s]", txID)
	loc, err := mgr.index.getTxLoc(txID)
	if err != nil {
		return nil, err
	}
	return mgr.fetchTransaction(loc)
}

func (mgr *blockfileMgr) fetchBlock(lp *fileLocPointer) (*protos.Block2, error) {
	serBlock, err := mgr.fetchSerBlock(lp)
	if err != nil {
		return nil, err
	}
	block, err := serBlock.ToBlock2()
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (mgr *blockfileMgr) fetchSerBlock(lp *fileLocPointer) (*protos.SerBlock2, error) {
	blockBytes, err := mgr.fetchBlockBytes(lp)
	if err != nil {
		return nil, err
	}
	return protos.NewSerBlock2(blockBytes), nil
}

func (mgr *blockfileMgr) fetchTransaction(lp *fileLocPointer) (*protos.Transaction2, error) {
	txBytes, err := mgr.fetchRawBytes(lp)
	if err != nil {
		return nil, err
	}
	tx := &protos.Transaction2{}
	err = proto.Unmarshal(txBytes, tx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (mgr *blockfileMgr) fetchBlockBytes(lp *fileLocPointer) ([]byte, error) {
	stream, err := newBlockfileStream(mgr.rootDir, lp.fileSuffixNum, int64(lp.offset))
	if err != nil {
		return nil, err
	}
	defer stream.close()
	b, err := stream.nextBlockBytes()
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (mgr *blockfileMgr) fetchRawBytes(lp *fileLocPointer) ([]byte, error) {
	filePath := deriveBlockfilePath(mgr.rootDir, lp.fileSuffixNum)
	reader, err := newBlockfileReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	b, err := reader.read(lp.offset, lp.bytesLength)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (mgr *blockfileMgr) loadCurrentInfo() (*checkpointInfo, error) {
	var b []byte
	var err error
	if b, err = mgr.db.Get(mgr.defaultCF, blkMgrInfoKey); b == nil || err != nil {
		return nil, err
	}
	i := &checkpointInfo{}
	if err = i.unmarshal(b); err != nil {
		return nil, err
	}
	logger.Debugf("loaded checkpointInfo:%s", i)
	return i, nil
}

func (mgr *blockfileMgr) saveCurrentInfo(i *checkpointInfo, flush bool) error {
	b, err := i.marshal()
	if err != nil {
		return err
	}
	if err = mgr.db.Put(mgr.defaultCF, blkMgrInfoKey, b); err != nil {
		return err
	}
	if flush {
		if err = mgr.db.Flush(true); err != nil {
			return err
		}
		logger.Debugf("saved checkpointInfo:%s", i)
	}
	return nil
}

func scanForLastCompleteBlock(rootDir string, fileNum int, startingOffset int64) (int64, int, error) {
	numBlocks := 0
	blockStream, err := newBlockfileStream(rootDir, fileNum, startingOffset)
	if err != nil {
		return 0, 0, err
	}
	defer blockStream.close()
	for {
		blockBytes, err := blockStream.nextBlockBytes()
		if blockBytes == nil || err == ErrUnexpectedEndOfBlockfile {
			logger.Debugf(`scanForLastCompleteBlock(): error=[%s].
			The error may happen if a crash has happened during block appending.
			Returning current offset as a last complete block's end offset`, err)
			break
		}
		numBlocks++
	}
	logger.Debugf("scanForLastCompleteBlock(): last complete block ends at offset=[%d]", blockStream.currentOffset)
	return blockStream.currentOffset, numBlocks, err
}

// checkpointInfo
type checkpointInfo struct {
	latestFileChunkSuffixNum int
	latestFileChunksize      int
	lastBlockNumber          uint64
}

func (i *checkpointInfo) marshal() ([]byte, error) {
	buffer := proto.NewBuffer([]byte{})
	var err error
	if err = buffer.EncodeVarint(uint64(i.latestFileChunkSuffixNum)); err != nil {
		return nil, err
	}
	if err = buffer.EncodeVarint(uint64(i.latestFileChunksize)); err != nil {
		return nil, err
	}
	if err = buffer.EncodeVarint(i.lastBlockNumber); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (i *checkpointInfo) unmarshal(b []byte) error {
	buffer := proto.NewBuffer(b)
	var val uint64
	var err error

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.latestFileChunkSuffixNum = int(val)

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.latestFileChunksize = int(val)

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.lastBlockNumber = val

	return nil
}

func (i *checkpointInfo) String() string {
	return fmt.Sprintf("latestFileChunkSuffixNum=[%d], latestFileChunksize=[%d], lastBlockNumber=[%d]",
		i.latestFileChunkSuffixNum, i.latestFileChunksize, i.lastBlockNumber)
}
