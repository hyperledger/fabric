/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	snapshotMetadataFileName     = "_snapshot_signable_metadata.json"
	snapshotMetadataHashFileName = "_snapshot_additional_info.json"
	jsonFileIndent               = "    "
)

// snapshotSignableMetadata is used to build a JSON that represents a unique snapshot and
// can be signed by the peer. Hashsum of the resultant JSON is intended to be used as a single
// hash of the snapshot, if need be.
type snapshotSignableMetadata struct {
	ChannelName        string            `json:"channel_name"`
	ChannelHeight      uint64            `json:"channel_height"`
	LastBlockHashInHex string            `json:"last_block_hash"`
	FilesAndHashes     map[string]string `json:"snapshot_files_raw_hashes"`
}

type snapshotAdditionalInfo struct {
	SnapshotHashInHex        string `json:"snapshot_hash"`
	LastBlockCommitHashInHex string `json:"last_block_commit_hash"`
}

// generateSnapshot generates a snapshot. This function should be invoked when commit on the kvledger are paused
// after committing the last block fully and further the commits should not be resumed till this function finishes
func (l *kvLedger) generateSnapshot() error {
	snapshotsRootDir := l.snapshotsConfig.RootDir
	bcInfo, err := l.GetBlockchainInfo()
	if err != nil {
		return err
	}
	snapshotTempDir, err := ioutil.TempDir(
		InProgressSnapshotsPath(snapshotsRootDir),
		fmt.Sprintf("%s-%d-", l.ledgerID, bcInfo.Height),
	)
	if err != nil {
		return errors.Wrapf(err, "error while creating temp dir [%s]", snapshotTempDir)
	}
	newHashFunc := func() (hash.Hash, error) {
		return l.hashProvider.GetHash(snapshotHashOpts)
	}
	txIDsExportSummary, err := l.blockStore.ExportTxIds(snapshotTempDir, newHashFunc)
	if err != nil {
		return err
	}
	configsHistoryExportSummary, err := l.configHistoryRetriever.ExportConfigHistory(snapshotTempDir, newHashFunc)
	if err != nil {
		return err
	}
	stateDBExportSummary, err := l.txmgr.ExportPubStateAndPvtStateHashes(snapshotTempDir, newHashFunc)
	if err != nil {
		return err
	}

	if err := l.generateSnapshotMetadataFiles(
		snapshotTempDir, txIDsExportSummary,
		configsHistoryExportSummary, stateDBExportSummary,
	); err != nil {
		return err
	}
	if err := syncDir(snapshotTempDir); err != nil {
		return err
	}
	slgr := SnapshotsDirForLedger(snapshotsRootDir, l.ledgerID)
	if err := os.MkdirAll(slgr, 0755); err != nil {
		return errors.Wrapf(err, "error while creating final dir for snapshot:%s", slgr)
	}
	if err := syncParentDir(slgr); err != nil {
		return err
	}
	slgrht := SnapshotDirForLedgerHeight(l.snapshotsConfig.RootDir, l.ledgerID, bcInfo.Height)
	if err := os.Rename(snapshotTempDir, slgrht); err != nil {
		return errors.Wrapf(err, "error while renaming dir [%s] to [%s]:", snapshotTempDir, slgrht)
	}
	return syncParentDir(slgrht)
}

func (l *kvLedger) generateSnapshotMetadataFiles(
	dir string,
	txIDsExportSummary,
	configsHistoryExportSummary,
	stateDBExportSummary map[string][]byte) error {
	// generate metadata file
	filesAndHashes := map[string]string{}
	for fileName, hashsum := range txIDsExportSummary {
		filesAndHashes[fileName] = hex.EncodeToString(hashsum)
	}
	for fileName, hashsum := range configsHistoryExportSummary {
		filesAndHashes[fileName] = hex.EncodeToString(hashsum)
	}
	for fileName, hashsum := range stateDBExportSummary {
		filesAndHashes[fileName] = hex.EncodeToString(hashsum)
	}
	bcInfo, err := l.GetBlockchainInfo()
	if err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(
		&snapshotSignableMetadata{
			ChannelName:        l.ledgerID,
			ChannelHeight:      bcInfo.Height,
			LastBlockHashInHex: hex.EncodeToString(bcInfo.CurrentBlockHash),
			FilesAndHashes:     filesAndHashes,
		},
		"",
		jsonFileIndent,
	)
	if err != nil {
		return errors.Wrap(err, "error while marshelling snapshot metadata to JSON")
	}
	if err := createAndSyncFile(filepath.Join(dir, snapshotMetadataFileName), metadata); err != nil {
		return err
	}

	// generate metadata hash file
	hash, err := l.hashProvider.GetHash(snapshotHashOpts)
	if err != nil {
		return err
	}
	if _, err := hash.Write(metadata); err != nil {
		return err
	}
	metadataAdditionalInfo, err := json.MarshalIndent(
		&snapshotAdditionalInfo{
			SnapshotHashInHex:        hex.EncodeToString(hash.Sum(nil)),
			LastBlockCommitHashInHex: hex.EncodeToString(l.commitHash),
		},
		"",
		jsonFileIndent,
	)
	if err != nil {
		return errors.Wrap(err, "error while marshalling snapshot additional info to JSON")
	}
	return createAndSyncFile(filepath.Join(dir, snapshotMetadataHashFileName), metadataAdditionalInfo)
}

func createAndSyncFile(filePath string, content []byte) error {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0444)
	if err != nil {
		return errors.Wrapf(err, "error while creating file:%s", filePath)
	}
	_, err = file.Write(content)
	if err != nil {
		file.Close()
		return errors.Wrapf(err, "error while writing to file:%s", filePath)
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return errors.Wrapf(err, "error while synching the file:%s", filePath)
	}
	if err := file.Close(); err != nil {
		return errors.Wrapf(err, "error while closing the file:%s", filePath)
	}
	return nil
}

func syncParentDir(path string) error {
	return syncDir(filepath.Dir(path))
}

func syncDir(dirPath string) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return errors.Wrapf(err, "error while opening dir:%s", dirPath)
	}
	if err := dir.Sync(); err != nil {
		dir.Close()
		return errors.Wrapf(err, "error while synching dir:%s", dirPath)
	}
	if err := dir.Close(); err != nil {
		return errors.Wrapf(err, "error while closing dir:%s", dirPath)
	}
	return err
}
