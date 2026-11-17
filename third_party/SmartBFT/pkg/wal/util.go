// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hyperledger/SmartBFT/pkg/api"
	protos "github.com/hyperledger/SmartBFT/smartbftprotos"
	"google.golang.org/protobuf/proto"
)

var padTable [][]byte

func init() {
	padTable = make([][]byte, 8)
	for i := range 8 {
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

	slices.Sort(indexes)

	return indexes, nil
}

func getPadSize(recordLength int) int {
	return (8 - recordLength%8) % 8
}

// crcAnchorRecordSize returns the on-disk size of the CRC-Anchor record that
// starts every WAL file.
func crcAnchorRecordSize() int64 {
	payloadLength := proto.Size(&protos.LogRecord{Type: protos.LogRecord_CRC_ANCHOR})

	return int64(recordHeaderSize + payloadLength + getPadSize(payloadLength))
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

// copyFile copies the source file to the target, readable and writable by the owner only,
// just like the WAL files themselves (walFilePermPrivateRW). Permissions of an already
// existing target are tightened before anything is written to it, since the open mode
// applies only when the file is created.
func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, walFilePermPrivateRW)
	if err != nil {
		return err
	}

	if err = output.Chmod(walFilePermPrivateRW); err != nil {
		_ = output.Close()
		return err
	}

	if _, err = io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}

	// The copy is the only evidence left once scanRepairFile truncates or
	// deletes the original, so flush it to disk before returning.
	if err = output.Sync(); err != nil {
		_ = output.Close()
		return err
	}

	return output.Close()
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
// read, the file is deleted only when it holds nothing but that anchor; otherwise the repair is
// refused with ErrNotRepairable.
//
// Only corruption of the last record is repaired. Anything else is refused with
// ErrNotRepairable, since truncating at a corrupt record in the middle of the file would
// drop records that were written before it, possibly ones that were already committed.
func scanRepairFile(logger api.Logger, lastFile string) error {
	logger.Debugf("Trying to repair file: %s", lastFile)

	r, err := NewLogRecordReader(logger, lastFile)
	if err != nil {
		logger.Warnf("Write-Ahead-Log could not open the last file, due to error: %s", err)

		// A torn CRC-Anchor is only possible while the file is being prepared,
		// before the first Append, so the file holds nothing but that anchor. If
		// it is larger, records were synced to it and deleting it would lose them.
		fileInfo, statErr := os.Stat(lastFile)
		if statErr != nil {
			return statErr
		}

		if fileInfo.Size() > crcAnchorRecordSize() {
			logger.Errorf(
				"Write-Ahead-Log will not repair file: %s; its CRC-Anchor is corrupt but the file holds records",
				lastFile,
			)

			return fmt.Errorf("%w: corrupt CRC-Anchor of file: %s", ErrNotRepairable, lastFile)
		}

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

			// offset is the end of the last good record, hence the start of the corrupt one.
			corruptRecordStart := offset

			if closeErr := r.Close(); closeErr != nil {
				return closeErr
			}

			if errors.Is(readErr, io.EOF) {
				logger.Debugf("Read %d good records till EOF, no need to repair", num)

				return nil // no need to repair
			}

			torn, err := isTornTail(logger, lastFile, corruptRecordStart)
			if err != nil {
				return err
			}

			if !torn {
				logger.Errorf(
					"Write-Ahead-Log will not repair file: %s; the corrupt record at offset %d is followed by further records, truncating it would drop them",
					lastFile,
					corruptRecordStart,
				)

				return fmt.Errorf(
					"%w: corrupt record at offset %d of file: %s",
					ErrNotRepairable,
					corruptRecordStart,
					lastFile,
				)
			}

			break
		}
		// keep the end offset of a good record
		num++

		offset, err = r.logFile.Seek(0, io.SeekCurrent)
		if err != nil {
			_ = r.Close()

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

// isTornTail reports whether the record starting at recordStart is the last one in the
// file, i.e. whether nothing was written after it.
//
// Repair truncates the file after the last good record, which is safe only for a torn
// write of the last record. If real records follow the corrupt one, they were written
// before the crash and may already be committed, so truncating would silently restart the
// node from a state in the past.
//
// The corrupt record cannot be parsed, so its length is unknown and the offset of the next
// record cannot be computed. When, however, only its payload or CRC is corrupt, the header
// still holds the true length, and the next record starts exactly after it - that position
// is probed first, and the first record there is verified against the corrupt header's CRC.
// A record that merely parses at that exact position is also treated as evidence that
// records follow. Otherwise we resynchronize in a single pass: every offset at which a
// record could start is probed once, and a remainder is accepted only if no readable link
// is found from any of them. Requiring a readable record at *some* offset keeps this
// conservative in the safe direction - a false positive refuses the repair, never performs
// a lossy one.
func isTornTail(logger api.Logger, fileName string, recordStart int64) (bool, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return false, err
	}
	defer f.Close()

	fileInfo, err := f.Stat()
	if err != nil {
		return false, err
	}

	fileSize := fileInfo.Size()
	if recordStart >= fileSize {
		return true, nil
	}

	// The corrupt record cannot be parsed. Read the rest of the file once and
	// resynchronize over the bytes in memory: probing every candidate offset
	// with its own ReadAt would issue one syscall per offset.
	tail := make([]byte, fileSize-recordStart)
	if _, err := f.ReadAt(tail, recordStart); err != nil {
		return false, err
	}

	var corruptCRC uint32
	if len(tail) >= recordHeaderSize {
		corruptCRC = uint32((binary.LittleEndian.Uint64(tail) & recordCRCMask) >> 32)
	}

	if next, ok := nextRecordOffset(tail); ok {
		// The corrupt header still holds the CRC that the next record chains
		// from, so a single following record can be verified. A record that
		// merely parses at the exact next offset is also enough: refusing the
		// repair is safe, truncating is not.
		if readableRecordsFollowFrom(tail, next, corruptCRC, true) || readableRecordAt(tail, next) {
			logger.Debugf(
				"Corrupt record at offset %d of file: %s is followed by readable records from offset %d",
				recordStart,
				fileName,
				recordStart+next,
			)

			return false, nil
		}
	}

	if readableLinkPresent(tail, corruptCRC) {
		logger.Debugf(
			"Corrupt record at offset %d of file: %s is followed by readable records",
			recordStart,
			fileName,
		)

		return false, nil
	}

	return true, nil
}

// readableLinkPresent reports whether data holds two consecutive readable
// records with a verified CRC link. The link is verified either against seedCRC,
// the CRC of the corrupt record before data, or between consecutive records.
// Records are interpreted as LogRecordReader.Read interprets them: an anchor
// reseeds the chain from its header CRC instead of being verified. Every aligned
// offset is parsed once, so the scan does not restart the chain per candidate.
func readableLinkPresent(data []byte, seedCRC uint32) bool {
	for pos := int64(recordHeaderSize); pos+int64(recordHeaderSize) <= int64(len(data)); pos += int64(recordHeaderSize) {
		current, ok := parseRecordAt(data, pos)
		if !ok {
			continue
		}

		if current.recordType != protos.LogRecord_CRC_ANCHOR && verifyCRC(seedCRC, current.crc, current.payload) {
			return true
		}

		next, ok := parseRecordAt(data, current.next)
		if !ok {
			continue
		}

		if next.recordType != protos.LogRecord_CRC_ANCHOR {
			if verifyCRC(current.crc, next.crc, next.payload) {
				return true
			}

			continue
		}

		afterAnchor, ok := parseRecordAt(data, next.next)
		if !ok || afterAnchor.recordType == protos.LogRecord_CRC_ANCHOR {
			continue
		}

		if verifyCRC(next.crc, afterAnchor.crc, afterAnchor.payload) {
			return true
		}
	}

	return false
}

// nextRecordOffset returns the offset, relative to the start of data, at which
// the record following the one at the start of data begins, based on the length
// in its header. It reports false when the header does not hold a length that
// leaves room for another record, i.e. when the header is corrupt too.
func nextRecordOffset(data []byte) (int64, bool) {
	if len(data) < recordHeaderSize {
		return 0, false
	}

	length := int64(binary.LittleEndian.Uint64(data) & recordLengthMask)
	payloadLength := length + int64(getPadSize(int(length)))

	next := int64(recordHeaderSize) + payloadLength
	if payloadLength == 0 || next+int64(recordHeaderSize) > int64(len(data)) {
		return 0, false
	}

	return next, true
}

type recordView struct {
	recordType protos.LogRecord_Type
	payload    []byte
	crc        uint32
	next       int64
}

// parseRecordAt parses the record starting at pos. It reports false when the
// record is torn, does not unmarshal, or has an unknown type.
func parseRecordAt(data []byte, pos int64) (recordView, bool) {
	var view recordView

	if pos+int64(recordHeaderSize) > int64(len(data)) {
		return view, false
	}

	header := binary.LittleEndian.Uint64(data[pos:])
	length := int64(header & recordLengthMask)
	view.crc = uint32((header & recordCRCMask) >> 32)

	payloadLength := length + int64(getPadSize(int(length)))
	if payloadLength == 0 || pos+int64(recordHeaderSize)+payloadLength > int64(len(data)) {
		return view, false
	}

	view.payload = data[pos+int64(recordHeaderSize) : pos+int64(recordHeaderSize)+payloadLength]

	record := &protos.LogRecord{}
	if err := proto.Unmarshal(view.payload[:length], record); err != nil {
		return view, false
	}

	switch record.GetType() {
	case protos.LogRecord_ENTRY, protos.LogRecord_CONTROL, protos.LogRecord_CRC_ANCHOR:
		view.recordType = record.GetType()
	default:
		return view, false
	}

	view.next = pos + int64(recordHeaderSize) + payloadLength

	return view, true
}

// readableRecordAt reports whether a structurally valid record starts at offset.
func readableRecordAt(data []byte, offset int64) bool {
	_, ok := parseRecordAt(data, offset)

	return ok
}

// readableRecordsFollow reports whether data holds readable records from offset.
// The CRC of the first record is not checked, since it depends on the corrupt
// record. One verified CRC link between consecutive records is enough; a lone
// record that merely decodes proves nothing.
func readableRecordsFollow(data []byte, offset int64) bool {
	return readableRecordsFollowFrom(data, offset, 0, false)
}

// readableRecordsFollowFrom reports whether data holds readable records from
// offset. When haveSeed is true, seedCRC is the CRC of the record before offset
// and the first record's CRC is verified against it. A failed link does not end
// the search: the current record becomes the new seed, so a later verified link
// still counts.
//
// Records are interpreted as LogRecordReader.Read interprets them: an anchor
// (CRC_ANCHOR) reseeds the chain from its header CRC instead of being verified,
// since its header CRC is the CRC of the previous record, not of its payload.
func readableRecordsFollowFrom(data []byte, offset int64, seedCRC uint32, haveSeed bool) bool {
	prevCRC := seedCRC

	for pos := offset; ; {
		record, ok := parseRecordAt(data, pos)
		if !ok {
			return false
		}

		switch record.recordType {
		case protos.LogRecord_CRC_ANCHOR:
			prevCRC = record.crc
			haveSeed = true
		case protos.LogRecord_ENTRY, protos.LogRecord_CONTROL:
			if haveSeed && verifyCRC(prevCRC, record.crc, record.payload) {
				return true
			}

			prevCRC = record.crc
			haveSeed = true
		}

		pos = record.next
	}
}
