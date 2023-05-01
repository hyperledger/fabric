// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package wal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SmartBFT-Go/consensus/pkg/api"
)

var padTable [][]byte

func init() {
	padTable = make([][]byte, 8)
	for i := 0; i < 8; i++ {
		padTable[i] = make([]byte, i)
	}
}

func dirEmpty(dirPath string) bool {
	names, err := dirReadWalNames(dirPath)
	if err != nil {
		return true
	}

	return len(names) == 0
}

func dirCreate(dirPath string) error {
	dirFile, err := os.Open(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(dirPath, walDirPermPrivateRWX)
		}

		return err
	}

	defer dirFile.Close()

	return err
}

// dirReadWalNames finds file names that follow the wal file name template.
func dirReadWalNames(dirPath string) ([]string, error) {
	dirFile, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer dirFile.Close()

	names, err := dirFile.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	walNames := make([]string, 0)

	for _, name := range names {
		if strings.HasSuffix(name, walFileSuffix) {
			var index uint64

			n, err := fmt.Sscanf(name, walFileTemplate, &index)
			if n != 1 || err != nil {
				continue
			}

			walNames = append(walNames, name)
		}
	}

	sort.Strings(walNames)

	return walNames, nil
}

// checkWalFiles for continuous sequence, readable CRC-Anchor.
// If the last file cannot be read, it may be ignored,  (or repaired).
func checkWalFiles(logger api.Logger, dirName string, walNames []string) ([]uint64, error) {
	sort.Strings(walNames)

	indexes := make([]uint64, 0)

	for i, name := range walNames {
		index, err := parseWalFileName(name)
		if err != nil {
			logger.Errorf("wal: failed to parse file name: %s; error: %s", name, err)

			return nil, err
		}

		indexes = append(indexes, index)

		// verify we have CRC-Anchor.
		r, err := NewLogRecordReader(logger, filepath.Join(dirName, walNames[i]))
		if err != nil {
			// check if it is the last file and return a special error that allows a repair.
			if i == len(walNames)-1 {
				logger.Errorf(
					"wal: failed to create reader for last file: %s; error: %s; this may possibly be repaired.",
					name,
					err,
				)

				return nil, io.ErrUnexpectedEOF
			}

			return nil, fmt.Errorf("wal: failed to create reader for file: %s; error: %w", name, err)
		}

		err = r.Close()
		if err != nil {
			return nil, fmt.Errorf("wal: failed to close reader for file: %s; error: %w", name, err)
		}

		// verify no gaps
		if i == 0 {
			continue
		}

		if index != (indexes[i-1] + 1) {
			return nil, errors.New("wal: files not in sequence")
		}
	}

	sort.Slice(indexes,
		func(i, j int) bool {
			return indexes[i] < indexes[j]
		},
	)

	return indexes, nil
}

func getPadSize(recordLength int) int {
	return (8 - recordLength%8) % 8
}

func getPadBytes(recordLength int) (int, []byte) {
	i := getPadSize(recordLength)

	return i, padTable[i]
}

func parseWalFileName(fileName string) (index uint64, err error) {
	n, err := fmt.Sscanf(fileName, walFileTemplate, &index)
	if n != 1 || err != nil {
		return 0, fmt.Errorf("failed to parse wal file name: %s; error: %w", fileName, err)
	}

	return index, nil
}

func copyFile(source, target string) error {
	input, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	return os.WriteFile(target, input, 0o644)
}

func truncateCloseFile(f *os.File, offset int64) error {
	if err := f.Truncate(offset); err != nil {
		return fmt.Errorf("failed to truncate at: %d; log file: %s; error: %w", offset, f.Name(), err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync log file: %s; error: %w", f.Name(), err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %s; error: %w", f.Name(), err)
	}

	return nil
}

// scanVerifyFiles.
func scanVerifyFiles(logger api.Logger, dirPath string, files []string) error {
	var (
		crc           uint32
		num, numTotal int
	)

	for i, name := range files {
		fullName := filepath.Join(dirPath, name)

		r, err := NewLogRecordReader(logger, fullName)
		if err != nil {
			return err
		}

		if num, err = scanVerify1(logger, r, i, crc); err != nil {
			return err
		}

		numTotal += num
		crc = r.crc // final CRC of file
	}

	logger.Debugf("Scanned %d records in %d files", numTotal, len(files))

	return nil
}

func scanVerify1(logger api.Logger, r *LogRecordReader, i int, crc uint32) (num int, err error) {
	defer func() { _ = r.Close() }()

	if i > 0 && crc != r.crc {
		logger.Errorf("Anchor-CRC %08X of file: %s, does not match previous: %08X", r.crc, r.logFile.Name(), crc)

		return 0, ErrCRC
	}

	var readErr error
	for readErr == nil {
		num++

		_, readErr = r.Read()
	}

	if !errors.Is(readErr, io.EOF) {
		return num - 1, readErr
	}

	return num - 1, nil
}

// scanRepairFile scans the file to the last good record and truncates after it. If even the CRC-Anchor cannot be
// read, the file is deleted.
func scanRepairFile(logger api.Logger, lastFile string) error {
	logger.Debugf("Trying to repair file: %s", lastFile)

	r, err := NewLogRecordReader(logger, lastFile)
	if err != nil {
		logger.Warnf("Write-Ahead-Log could not open the last file, due to error: %s", err)

		if err = os.Remove(lastFile); err != nil {
			return err
		}

		logger.Warnf("Write-Ahead-Log DELETED the last file (a copy was saved): %s", lastFile)

		return nil
	}

	// scan to the last good record
	offset, err := r.logFile.Seek(0, io.SeekCurrent)
	if err != nil {
		_ = r.Close()

		return err
	}

	num := 0

	for {
		_, readErr := r.Read()
		if readErr != nil {
			logger.Debugf("Read error: %s", readErr)

			if closeErr := r.Close(); closeErr != nil {
				return closeErr
			}

			if errors.Is(readErr, io.EOF) {
				logger.Debugf("Read %d good records till EOF, no need to repair", num)

				return nil // no need to repair
			}

			break
		}
		// keep the end offset of a good record
		num++

		offset, err = r.logFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
	}

	logger.Debugf("Read %d good records, last good record ended at offset: %d", num, offset)

	f, err := os.OpenFile(lastFile, os.O_RDWR, walFilePermPrivateRW)
	if err != nil {
		logger.Errorf("Failed to open log file: %s; error: %s", lastFile, err)

		return err
	}

	if err = truncateCloseFile(f, offset); err != nil {
		logger.Errorf("Failed to truncateCloseFile: error: %s", err)

		return err
	}

	return nil
}
