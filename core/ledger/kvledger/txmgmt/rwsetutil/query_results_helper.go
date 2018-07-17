/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	bccspfactory "github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

// MerkleTreeLevel used for representing a level of the merkle tree
type MerkleTreeLevel uint32

// Hash represents bytes of a hash
type Hash []byte

const (
	leafLevel = MerkleTreeLevel(1)
)

var (
	hashOpts = &bccsp.SHA256Opts{}
)

// RangeQueryResultsHelper helps preparing range query results for phantom items detection during validation.
// The results are expected to be fed as they are being iterated over.
// If the `hashingEnabled` is set to true, a merkle tree is built of the hashes over the results.
// The merkle tree helps reducing the size of the RWSet which otherwise would need to store all the raw KVReads
//
// The mental model of the tree can be described as below:
// All the results are treated as leaf nodes (level 0) of the tree. Next up level of the tree is built by collecting 'maxDegree + 1'
// items from the previous level and hashing the entire collection.
// Further upper levels of the tree are built in similar manner however the only difference is that unlike level-0
// (where collection consists of raw KVReads), collection at level 1 and above, consists of the hashes
// (of the collection of previous level).
// This is repeated until we reach at a level where we are left with the number of items less than or equals to `maxDegree`.
// In the last collection, the number of items can be less than 'maxDegree' (except if this is the only collection at the given level).
//
// As a result, if the number of total input results are less than or equals to 'maxDegree', no hashing is performed at all.
// And the final output of the computation is either the collection of raw results (if less that or equals to 'maxDegree') or
// a collection of hashes (that or equals to 'maxDegree') at some level in the tree.
//
// `AddResult` function should be invoke to supply the next result and at the end `Done` function should be invoked.
// The `Done` function does the final processing and returns the final output
type RangeQueryResultsHelper struct {
	pendingResults []*kvrwset.KVRead
	mt             *merkleTree
	maxDegree      uint32
	hashingEnabled bool
}

// NewRangeQueryResultsHelper constructs a RangeQueryResultsHelper
func NewRangeQueryResultsHelper(enableHashing bool, maxDegree uint32) (*RangeQueryResultsHelper, error) {
	helper := &RangeQueryResultsHelper{pendingResults: nil,
		hashingEnabled: enableHashing,
		maxDegree:      maxDegree,
		mt:             nil}
	if enableHashing {
		var err error
		if helper.mt, err = newMerkleTree(maxDegree); err != nil {
			return nil, err
		}
	}
	return helper, nil
}

// AddResult adds a new query result for processing.
// Put the result into the list of pending results. If the number of pending results exceeds `maxDegree`,
// consume the results for incrementally update the merkle tree
func (helper *RangeQueryResultsHelper) AddResult(kvRead *kvrwset.KVRead) error {
	logger.Debug("Adding a result")
	helper.pendingResults = append(helper.pendingResults, kvRead)
	if helper.hashingEnabled && uint32(len(helper.pendingResults)) > helper.maxDegree {
		logger.Debug("Processing the accumulated results")
		if err := helper.processPendingResults(); err != nil {
			return err
		}
	}
	return nil
}

// Done processes any pending results if needed
// This returns the final pending results (i.e., []*KVRead) and hashes of the results (i.e., *MerkleSummary)
// Only one of these two will be non-nil (except when no results are ever added).
// `MerkleSummary` will be nil if and only if either `enableHashing` is set to false
// or the number of total results are less than `maxDegree`
func (helper *RangeQueryResultsHelper) Done() ([]*kvrwset.KVRead, *kvrwset.QueryReadsMerkleSummary, error) {
	// The merkle tree will be empty if total results are less than or equals to 'maxDegree'
	// i.e., not even once the results were processed for hashing
	if !helper.hashingEnabled || helper.mt.isEmpty() {
		return helper.pendingResults, nil, nil
	}
	if len(helper.pendingResults) != 0 {
		logger.Debug("Processing the pending results")
		if err := helper.processPendingResults(); err != nil {
			return helper.pendingResults, nil, err
		}
	}
	helper.mt.done()
	return helper.pendingResults, helper.mt.getSummery(), nil
}

// GetMerkleSummary return the current state of the MerkleSummary
// This intermediate state of the merkle tree helps during validation to detect a mismatch early on.
// That helps by not requiring to build the complete merkle tree during validation
// if there is a mismatch in early portion of the result-set.
func (helper *RangeQueryResultsHelper) GetMerkleSummary() *kvrwset.QueryReadsMerkleSummary {
	if !helper.hashingEnabled {
		return nil
	}
	return helper.mt.getSummery()
}

func (helper *RangeQueryResultsHelper) processPendingResults() error {
	var b []byte
	var err error
	if b, err = serializeKVReads(helper.pendingResults); err != nil {
		return err
	}
	helper.pendingResults = nil
	hash, err := bccspfactory.GetDefault().Hash(b, hashOpts)
	if err != nil {
		return err
	}
	helper.mt.update(hash)
	return nil
}

func serializeKVReads(kvReads []*kvrwset.KVRead) ([]byte, error) {
	return proto.Marshal(&kvrwset.QueryReads{KvReads: kvReads})
}

//////////// Merkle tree building code  ///////

type merkleTree struct {
	tree      map[MerkleTreeLevel][]Hash
	maxLevel  MerkleTreeLevel
	maxDegree uint32
}

func newMerkleTree(maxDegree uint32) (*merkleTree, error) {
	if maxDegree < 2 {
		return nil, errors.Errorf("maxDegree [%d] should not be less than 2 in the merkle tree", maxDegree)
	}
	return &merkleTree{make(map[MerkleTreeLevel][]Hash), 1, maxDegree}, nil
}

// update takes a hash that forms the next leaf level (level-1) node in the merkle tree.
// Also, complete the merkle tree as much as possible with the addition of this new leaf node -
// i.e. recursively build the higher level nodes and delete the underlying sub-tree.
func (m *merkleTree) update(nextLeafLevelHash Hash) error {
	logger.Debugf("Before update() = %s", m)
	defer logger.Debugf("After update() = %s", m)
	m.tree[leafLevel] = append(m.tree[leafLevel], nextLeafLevelHash)
	currentLevel := leafLevel
	for {
		currentLevelHashes := m.tree[currentLevel]
		if uint32(len(currentLevelHashes)) <= m.maxDegree {
			return nil
		}
		nextLevelHash, err := computeCombinedHash(currentLevelHashes)
		if err != nil {
			return err
		}
		delete(m.tree, currentLevel)
		nextLevel := currentLevel + 1
		m.tree[nextLevel] = append(m.tree[nextLevel], nextLevelHash)
		if nextLevel > m.maxLevel {
			m.maxLevel = nextLevel
		}
		currentLevel = nextLevel
	}
}

// done completes the merkle tree.
// There may have been some nodes that are at the levels lower than the maxLevel (maximum level seen by the tree so far).
// Make the parent nodes out of such nodes till we complete the tree at the level of maxLevel (or maxLevel+1).
func (m *merkleTree) done() error {
	logger.Debugf("Before done() = %s", m)
	defer logger.Debugf("After done() = %s", m)
	currentLevel := leafLevel
	var h Hash
	var err error
	for currentLevel < m.maxLevel {
		currentLevelHashes := m.tree[currentLevel]
		switch len(currentLevelHashes) {
		case 0:
			currentLevel++
			continue
		case 1:
			h = currentLevelHashes[0]
		default:
			if h, err = computeCombinedHash(currentLevelHashes); err != nil {
				return err
			}
		}
		delete(m.tree, currentLevel)
		currentLevel++
		m.tree[currentLevel] = append(m.tree[currentLevel], h)
	}

	finalHashes := m.tree[m.maxLevel]
	if uint32(len(finalHashes)) > m.maxDegree {
		delete(m.tree, m.maxLevel)
		m.maxLevel++
		combinedHash, err := computeCombinedHash(finalHashes)
		if err != nil {
			return err
		}
		m.tree[m.maxLevel] = []Hash{combinedHash}
	}
	return nil
}

func (m *merkleTree) getSummery() *kvrwset.QueryReadsMerkleSummary {
	return &kvrwset.QueryReadsMerkleSummary{MaxDegree: m.maxDegree,
		MaxLevel:       uint32(m.getMaxLevel()),
		MaxLevelHashes: hashesToBytes(m.getMaxLevelHashes())}
}

func (m *merkleTree) getMaxLevel() MerkleTreeLevel {
	return m.maxLevel
}

func (m *merkleTree) getMaxLevelHashes() []Hash {
	return m.tree[m.maxLevel]
}

func (m *merkleTree) isEmpty() bool {
	return m.maxLevel == 1 && len(m.tree[m.maxLevel]) == 0
}

func (m *merkleTree) String() string {
	return fmt.Sprintf("tree := %#v", m.tree)
}

func computeCombinedHash(hashes []Hash) (Hash, error) {
	combinedHash := []byte{}
	for _, h := range hashes {
		combinedHash = append(combinedHash, h...)
	}
	return bccspfactory.GetDefault().Hash(combinedHash, hashOpts)
}

func hashesToBytes(hashes []Hash) [][]byte {
	b := [][]byte{}
	for _, hash := range hashes {
		b = append(b, hash)
	}
	return b
}
