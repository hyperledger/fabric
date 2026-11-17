// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/hyperledger/SmartBFT/pkg/api"
	protos "github.com/hyperledger/SmartBFT/smartbftprotos"
	"google.golang.org/protobuf/proto"
)

type LogRecordReader struct {
	fileName string
	logger   api.Logger
	logFile  *os.File
	crc      uint32
	// fileSizeBytes is the size of the log file. A record never spans beyond
	// the file it is in, so it bounds the length declared in a record header
	// and prevents a corrupted or forged header from driving a huge allocation.
	fileSizeBytes int64
}

func NewLogRecordReader(logger api.Logger, fileName string) (*LogRecordReader, error) {
	if logger == nil {
		return nil, errors.New("logger is nil")
	}

	r := &LogRecordReader{
		fileName: fileName,
		logger:   logger,
	}

	var err error

	r.logFile, err = os.Open(fileName)
	if err != nil {
		return nil, err
	}

	fileInfo, err := r.logFile.Stat()
	if err != nil {
		_ = r.Close()

		return nil, err
	}

	r.fileSizeBytes = fileInfo.Size()

	_, err = r.logFile.Seek(0, io.SeekStart)
	if err != nil {
		_ = r.Close()

		return nil, err
	}

	// read the CRC-Anchor, the first record of every file
	recLen, crc, err := r.readHeader()
	if err != nil {
		_ = r.Close()

		return nil, err
	}

	padSize := getPadSize(int(recLen))

	payload, err := r.readPayload(int(recLen) + padSize)
	if err != nil {
		_ = r.Close()

		return nil, err
	}

	record := &protos.LogRecord{}

	err = proto.Unmarshal(payload[:recLen], record)
	if err != nil {
		_ = r.Close()

		return nil, err
	}

	if record.GetType() != protos.LogRecord_CRC_ANCHOR {
		_ = r.Close()

		return nil, fmt.Errorf("failed reading CRC-Anchor from log file: %s", fileName)
	}

	r.crc = crc

	r.logger.Debugf("Initialized reader: CRC-Anchor: %08X, file: %s", r.crc, r.fileName)

	return r, nil
}

func (r *LogRecordReader) Close() error {
	var err error
	if r.logFile != nil {
		err = r.logFile.Close()
	}

	r.logger.Debugf("Closed reader: CRC: %08X, file: %s", r.crc, r.fileName)

	r.logger = nil
	r.logFile = nil

	return err
}

func (r *LogRecordReader) CRC() uint32 {
	return r.crc
}

func (r *LogRecordReader) Read() (*protos.LogRecord, error) {
	recLen, crc, err := r.readHeader()
	if err != nil {
		return nil, err
	}

	padSize := getPadSize(int(recLen))

	payload, err := r.readPayload(int(recLen) + padSize)
	if err != nil {
		return nil, err
	}

	record := &protos.LogRecord{}

	err = proto.Unmarshal(payload[:recLen], record)
	if err != nil {
		return nil, ErrWALUnmarshalPayload // fmt.Errorf("wal: failed to unmarshal payload: %w", err)
	}

	switch record.GetType() {
	case protos.LogRecord_ENTRY, protos.LogRecord_CONTROL:
		if !verifyCRC(r.crc, crc, payload) {
			return nil, ErrCRC
		}

		fallthrough
	case protos.LogRecord_CRC_ANCHOR:
		r.crc = crc
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnexpectedRecordType, record.GetType())
	}

	return record, nil
}

// readHeader attempts to read the 8 byte header.
// If it fails, it fails like io.ReadFull().
func (r *LogRecordReader) readHeader() (length, crc uint32, err error) {
	buff := make([]byte, recordHeaderSize)

	n, err := io.ReadFull(r.logFile, buff)
	if err != nil {
		r.logger.Debugf("Failed to read header in full: expected=%d, actual=%d; error: %s", recordHeaderSize, n, err)

		return 0, 0, err
	}

	header := binary.LittleEndian.Uint64(buff)
	length = uint32(header & recordLengthMask)
	crc = uint32((header & recordCRCMask) >> 32)

	// A record, together with its header, must fit in the remaining part of the
	// file. Otherwise a corrupted header, especially near the end of the file,
	// makes us allocate up to the whole file size before the read fails.
	pos, err := r.logFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, err
	}

	recSize := int64(length) + int64(getPadSize(int(length)))

	if remaining := r.fileSizeBytes - pos; recSize > remaining {
		r.logger.Warnf("Record length %d in file: %s exceeds the remaining file size %d", length, r.fileName, remaining)

		// A header claiming more than the whole file can hold is corrupt. A
		// record that only outgrows what is left is a torn tail.
		if recSize > r.fileSizeBytes-int64(recordHeaderSize) {
			return 0, 0, fmt.Errorf("%w: length=%d, file size=%d, file: %s", ErrRecordTooBig, length, r.fileSizeBytes, r.fileName)
		}

		return 0, 0, io.ErrUnexpectedEOF
	}

	return length, crc, nil
}

// readPayload attempts to read a payload in full.
// If it fails, it fails like io.ReadFull().
func (r *LogRecordReader) readPayload(len int) (payload []byte, err error) {
	buff := make([]byte, len)

	n, err := io.ReadFull(r.logFile, buff)
	if err != nil {
		r.logger.Debugf("Failed to read payload in full: expected=%d, actual=%d; error: %s", len, n, err)

		return nil, err
	}

	return buff, nil
}

func verifyCRC(prevCRC, expectedCRC uint32, data []byte) bool {
	dataCRC := crc32.Update(prevCRC, crcTable, data)

	return dataCRC == expectedCRC
}
