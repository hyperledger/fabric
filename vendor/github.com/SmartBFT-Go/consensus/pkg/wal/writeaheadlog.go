// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/metrics/disabled"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	walFileSuffix   string = ".wal"
	walFileTemplate        = "%016x" + walFileSuffix

	walFilePermPrivateRW os.FileMode = 0o600
	walDirPermPrivateRWX os.FileMode = 0o700

	recordHeaderSize int    = 8
	recordLengthMask uint64 = 0x00000000FFFFFFFF
	recordCRCMask           = recordLengthMask << 32

	walCRCSeed uint32 = 0xDEED0001

	FileSizeBytesDefault   int64 = 64 * 1024 * 1024 // 64MB
	BufferSizeBytesDefault int64 = 1024 * 1024      // 1MB
)

var (
	ErrCRC                 = errors.New("wal: crc verification failed")
	ErrWALUnmarshalPayload = errors.New("wal: failed to unmarshal payload")
	ErrWriteOnly           = errors.New("wal: in WRITE mode")
	ErrReadOnly            = errors.New("wal: in READ mode")

	ErrWALAlreadyExists = errors.New("wal: is already exists")

	crcTable = crc32.MakeTable(crc32.Castagnoli)
)

type LogRecordLength uint32

type LogRecordCRC uint32

// LogRecordHeader contains the LogRecordLength (lower 32 bits) and LogRecordCRC (upper 32 bits).
type LogRecordHeader uint64

// WriteAheadLogFile is a simple implementation of a write ahead log (WAL).
//
// The WAL is composed of a sequence of frames. Each frame contains:
// - a header (uint64)
// - payload: a record of type LogRecord, marshaled to bytes, and padded with zeros to 8B boundary.
//
// The 64 bit header is made of two parts:
// - length of the marshaled LogRecord (not including pad bytes), in the lower 32 bits.
// - a crc32 of the data: marshaled record bytes + pad bytes, in the upper 32 bits.
//
// The WAL is written to a sequence of files: <index>.wal, where index uint64=1,2,3...; represented in fixed-width
// hex format, e.g. 0000000000000001.wal
//
// The WAL has two modes: append, and read.
//
// When a WAL is first created, it is in append mode.
// When an existing WAL is opened, it is in read mode, and will change to append mode only after ReadAll() is invoked.
//
// In append mode the WAL can accept Append() and TruncateTo() calls.
// The WAL must be closed after use to release all resources.
type WriteAheadLogFile struct {
	dirName string
	options *Options

	logger  api.Logger
	metrics *Metrics

	mutex         sync.Mutex
	dirFile       *os.File
	index         uint64
	logFile       *os.File
	headerBuff    []byte
	dataBuff      *proto.Buffer
	crc           uint32
	readMode      bool
	truncateIndex uint64
	activeIndexes []uint64
}

type Options struct {
	FileSizeBytes   int64
	BufferSizeBytes int64
	MetricsProvider *api.CustomerProvider
}

// DefaultOptions returns the set of default options.
func DefaultOptions() *Options {
	return &Options{
		FileSizeBytes:   FileSizeBytesDefault,
		BufferSizeBytes: BufferSizeBytesDefault,
		MetricsProvider: api.NewCustomerProvider(&disabled.Provider{}),
	}
}

func (o *Options) String() string {
	return fmt.Sprintf("{FileSizeBytes: %d, BufferSizeBytes: %d}", o.FileSizeBytes, o.BufferSizeBytes)
}

// Create will create a new WAL, if it does not exist, or an error if it already exists.
//
// logger: reference to a Logger implementation.
// dirPath: directory path of the WAL.
// options: a structure containing Options, or nil, for default options.
//
// return: pointer to a WAL, ErrWALAlreadyExists if WAL already exists or other errors
func Create(logger api.Logger, dirPath string, options *Options) (*WriteAheadLogFile, error) {
	if logger == nil {
		return nil, errors.New("wal: logger is nil")
	}

	if !dirEmpty(dirPath) {
		return nil, ErrWALAlreadyExists
	}

	opt := DefaultOptions()
	if options != nil {
		if options.MetricsProvider != nil {
			opt.MetricsProvider = options.MetricsProvider
		}
		if options.FileSizeBytes != 0 {
			opt.FileSizeBytes = options.FileSizeBytes
		}
		if options.BufferSizeBytes != 0 {
			opt.BufferSizeBytes = options.BufferSizeBytes
		}
	}

	// TODO BACKLOG: create the directory & file atomically by creation in a temp dir and renaming
	cleanDirName := filepath.Clean(dirPath)

	err := dirCreate(cleanDirName)
	if err != nil {
		return nil, fmt.Errorf("wal: could not create directory: %s; error: %w", dirPath, err)
	}

	wal := &WriteAheadLogFile{
		dirName:       cleanDirName,
		options:       opt,
		logger:        logger,
		metrics:       NewMetrics(opt.MetricsProvider),
		index:         1,
		headerBuff:    make([]byte, 8),
		dataBuff:      proto.NewBuffer(make([]byte, opt.BufferSizeBytes)),
		crc:           walCRCSeed,
		truncateIndex: 1,
		activeIndexes: []uint64{1},
	}
	wal.metrics.CountOfFiles.Set(float64(len(wal.activeIndexes)))

	wal.dirFile, err = os.Open(cleanDirName)
	if err != nil {
		_ = wal.Close()

		return nil, fmt.Errorf("wal: could not open directory: %s; error: %w", dirPath, err)
	}

	fileName := fmt.Sprintf(walFileTemplate, uint64(1))

	wal.logFile, err = os.OpenFile(filepath.Join(cleanDirName, fileName), os.O_CREATE|os.O_WRONLY, walFilePermPrivateRW)
	if err != nil {
		_ = wal.Close()

		return nil, fmt.Errorf("wal: could not open file: %s; error: %w", fileName, err)
	}

	if err = wal.saveCRC(); err != nil {
		_ = wal.Close()

		return nil, err
	}

	wal.logger.Infof("Write-Ahead-Log created successfully, mode: WRITE, dir: %s", wal.dirName)

	return wal, nil
}

// Open will open an existing WAL, if it exists, or an error if it does not exist.
//
// After opening, the WAL is in read mode, and expects a call to ReadAll(). An attempt to write
// (e.g. Append, TruncateTo) will result in an error.
//
// logger: reference to a Logger implementation.
// dirPath: directory path of the WAL.
// options: a structure containing Options, or nil, for default options.
//
// return: pointer to a WAL, or an error
func Open(logger api.Logger, dirPath string, options *Options) (*WriteAheadLogFile, error) {
	if logger == nil {
		return nil, errors.New("wal: logger is nil")
	}

	walNames, err := dirReadWalNames(dirPath)
	if err != nil {
		return nil, err
	}

	if len(walNames) == 0 {
		return nil, os.ErrNotExist
	}

	logger.Infof("Write-Ahead-Log discovered %d wal files: %s", len(walNames), strings.Join(walNames, ", "))

	opt := DefaultOptions()
	if options != nil {
		if options.MetricsProvider != nil {
			opt.MetricsProvider = options.MetricsProvider
		}
		if options.FileSizeBytes != 0 {
			opt.FileSizeBytes = options.FileSizeBytes
		}
		if options.BufferSizeBytes != 0 {
			opt.BufferSizeBytes = options.BufferSizeBytes
		}
	}

	cleanDirName := filepath.Clean(dirPath)

	wal := &WriteAheadLogFile{
		dirName:    cleanDirName,
		options:    opt,
		logger:     logger,
		metrics:    NewMetrics(opt.MetricsProvider),
		headerBuff: make([]byte, 8),
		dataBuff:   proto.NewBuffer(make([]byte, opt.BufferSizeBytes)),
		readMode:   true,
	}

	wal.dirFile, err = os.Open(cleanDirName)
	if err != nil {
		_ = wal.Close()

		return nil, fmt.Errorf("wal: could not open directory: %s; error: %w", dirPath, err)
	}

	// After the check we have an increasing, continuous sequence, with valid CRC-Anchors in each file.
	wal.activeIndexes, err = checkWalFiles(logger, dirPath, walNames)
	if err != nil {
		wal.metrics.CountOfFiles.Set(float64(len(wal.activeIndexes)))
		_ = wal.Close()

		return nil, err
	}
	wal.metrics.CountOfFiles.Set(float64(len(wal.activeIndexes)))

	wal.index, err = parseWalFileName(walNames[0]) // first valid file
	if err != nil {
		_ = wal.Close()

		return nil, err
	}

	wal.logger.Infof("Write-Ahead-Log opened successfully, mode: READ, dir: %s", wal.dirName)

	return wal, nil
}

// Repair tries to repair the last file of a WAL, in case an Open() comes back with a io.ErrUnexpectedEOF,
// which indicates the possibility to fix the WAL.
//
// After a crash, the last log file in the WAL may be left in a state where an attempt tp reopen will result in
// the last Read() returning an error. This is because of several reasons:
// - we use pre-allocated / recycled files, the files have a "garbage" tail
// - the last write may have been torn
// - the failure might have occurred when the log file was being prepared (no anchor)
//
// The Repair() tries to repair the last file of the wal by truncating after the last good record.
// Before doing so it copies the bad file to a side location for later analysis by operators.
//
// logger: reference to a Logger implementation.
// dirPath: directory path of the WAL.
// return: an error if repair was not successful.
func Repair(logger api.Logger, dirPath string) error {
	cleanDirPath := filepath.Clean(dirPath)

	walNames, err := dirReadWalNames(cleanDirPath)
	if err != nil {
		return err
	}

	if len(walNames) == 0 {
		return os.ErrNotExist
	}

	logger.Infof("Write-Ahead-Log discovered %d wal files: %s", len(walNames), strings.Join(walNames, ", "))

	// verify that all but the last are fine
	if err = scanVerifyFiles(logger, cleanDirPath, walNames[:len(walNames)-1]); err != nil {
		logger.Errorf("Write-Ahead-Log failed to repair, additional files are faulty: %s", err)

		return err
	}

	lastFile := filepath.Join(cleanDirPath, walNames[len(walNames)-1])
	logger.Infof("Write-Ahead-Log is going to try and repair the last file: %s", lastFile)
	lastFileCopy := lastFile + ".copy"

	err = copyFile(lastFile, lastFileCopy)
	if err != nil {
		logger.Errorf("Write-Ahead-Log failed to repair, could not make a copy: %s", err)

		return err
	}

	logger.Infof("Write-Ahead-Log made a copy of the last file: %s", lastFileCopy)

	err = scanRepairFile(logger, lastFile)
	if err != nil {
		logger.Errorf("Write-Ahead-Log failed to scan and repair last file: %s", err)

		return err
	}

	logger.Infof("Write-Ahead-Log successfully repaired the last file: %s", lastFile)

	return nil
}

// Close the files and directory of the WAL, and release all resources.
func (w *WriteAheadLogFile) Close() error {
	var errF, errD error

	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.logFile != nil {
		if errF = w.truncateAndCloseLogFile(); errF != nil {
			w.logger.Errorf("failed to properly close log file %s; error: %s", w.logFile.Name(), errF)
		}

		w.logFile = nil
	}

	w.dataBuff = nil
	w.headerBuff = nil

	if w.dirFile != nil {
		if errD = w.dirFile.Close(); errD != nil {
			w.logger.Errorf("failed to properly close directory %s; error: %s", w.dirName, errD)
		}

		w.dirFile = nil
	}

	// return the first error
	switch {
	case errF != nil:
		return errF
	default:
		return errD
	}
}

// CRC returns the last CRC written to the log file.
func (w *WriteAheadLogFile) CRC() uint32 {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.crc
}

// TruncateTo appends a control record in which the TruncateTo flag is true.
// This marks that every record prior to this one can be safely truncated from the log.
func (w *WriteAheadLogFile) TruncateTo() error {
	record := &protos.LogRecord{
		Type:       protos.LogRecord_CONTROL,
		TruncateTo: true,
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.append(record)
}

// Append a data item to the end of the WAL and indicate whether this entry is a truncation point.
//
// The data item will be added to the log, and internally marked with a flag that indicates whether
// it is a truncation point. The log implementation may truncate all preceding data items, not including this one.
//
// data: the data to be appended to the log. Cannot be nil or empty.
// truncateTo: whether all records preceding this one, but not including it, can be truncated from the log.
func (w *WriteAheadLogFile) Append(data []byte, truncateTo bool) error {
	if len(data) == 0 {
		return errors.New("data is nil or empty")
	}

	record := &protos.LogRecord{
		Type:       protos.LogRecord_ENTRY,
		TruncateTo: truncateTo,
		Data:       data,
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.append(record)
}

func (w *WriteAheadLogFile) append(record *protos.LogRecord) error {
	if w.dirFile == nil {
		return os.ErrClosed
	}

	if w.readMode {
		return ErrReadOnly
	}

	w.dataBuff.Reset()

	err := w.dataBuff.Marshal(record)
	if err != nil {
		return fmt.Errorf("wal: failed to marshal to data buffer: %w", err)
	}

	payloadBuff := w.dataBuff.Bytes()

	recordLength := len(payloadBuff)
	if (uint64(recordLength) & recordCRCMask) != 0 {
		return fmt.Errorf("wal: record too big, length does not fit in uint32: %d", recordLength)
	}

	padSize, padBytes := getPadBytes(recordLength)
	if padSize != 0 {
		payloadBuff = append(payloadBuff, padBytes...)
	}

	dataCRC := crc32.Update(w.crc, crcTable, payloadBuff)
	header := uint64(recordLength) | (uint64(dataCRC) << 32)

	binary.LittleEndian.PutUint64(w.headerBuff, header)

	nh, err := w.logFile.Write(w.headerBuff)
	if err != nil {
		return fmt.Errorf("wal: failed to write header bytes: %w", err)
	}

	np, err := w.logFile.Write(payloadBuff)
	if err != nil {
		return fmt.Errorf("wal: failed to write payload bytes: %w", err)
	}

	err = w.logFile.Sync()
	if err != nil {
		return fmt.Errorf("wal: failed to Sync log file: %w", err)
	}

	w.crc = dataCRC

	offset, err := w.logFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("wal: failed to get offset from log file: %w", err)
	}

	if record.TruncateTo {
		w.truncateIndex = w.index
	}

	w.logger.Debugf(
		"LogRecord appended successfully: total size=%d, recordLength=%d, dataCRC=%08X; file=%s, new-offset=%d",
		nh+np,
		recordLength,
		dataCRC,
		w.logFile.Name(),
		offset,
	)

	// Switch files if this or the next record (minimal size is 16B) cause overflow
	if offset > w.options.FileSizeBytes-16 {
		err = w.switchFiles()
		if err != nil {
			return fmt.Errorf("wal: failed to switch log files: %w", err)
		}
	}

	return nil
}

// ReadAll the data items from the latest truncation point to the end of the log.
// This method can be called only at the beginning of the WAL lifecycle, right after Open().
// After a successful invocation the WAL moves to write mode, and is ready to Append().
//
// In case of failure:
//   - an error of type io.ErrUnexpectedEOF	is returned when the WAL can possibly be repaired by truncating the last
//     log file after the last good record.
//   - all other errors indicate that the WAL is either
//   - is closed, or
//   - is in write mode, or
//   - is corrupted beyond the simple repair measure described above.
func (w *WriteAheadLogFile) ReadAll() ([][]byte, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.dirFile == nil {
		return nil, os.ErrClosed
	}

	if !w.readMode {
		return nil, ErrWriteOnly
	}

	items := make([][]byte, 0)
	lastIndex := w.activeIndexes[len(w.activeIndexes)-1]

FileLoop:
	for i, index := range w.activeIndexes {
		w.index = index
		// This should not fail, we check the files earlier, when we Open() the WAL.
		r, err := NewLogRecordReader(w.logger, filepath.Join(w.dirName, fmt.Sprintf(walFileTemplate, w.index)))
		if err != nil {
			return nil, err
		}
		if (i != 0) && (r.CRC() != w.crc) {
			return nil, ErrCRC
		}

		var readErr error
	ReadLoop:
		for i := 1; ; i++ {
			var rec *protos.LogRecord
			rec, readErr = r.Read()
			if readErr != nil {
				w.logger.Debugf("Read error, file: %s; error: %s", r.fileName, readErr)
				_ = r.Close()

				break ReadLoop
			}

			if rec.TruncateTo {
				items = items[0:0]
				w.truncateIndex = w.index
			}

			if rec.Type == protos.LogRecord_ENTRY {
				items = append(items, rec.Data)
			}

			w.logger.Debugf("Read record #%d, file: %s", i, r.fileName)
		}

		if errors.Is(readErr, io.EOF) {
			w.logger.Debugf("Reached EOF, finished reading file: %s; CRC: %08X", r.fileName, r.CRC())
			w.crc = r.CRC()

			continue FileLoop
		}

		if index == lastIndex &&
			(errors.Is(readErr, io.ErrUnexpectedEOF) ||
				errors.Is(readErr, ErrCRC) ||
				errors.Is(readErr, ErrWALUnmarshalPayload)) {
			w.logger.Warnf(
				"Received an error in the last file, this can possibly be repaired; file: %s; error: %s",
				r.fileName,
				readErr,
			)
			// This error is returned when the WAL can possibly be repaired
			return nil, io.ErrUnexpectedEOF
		}

		if readErr != nil {
			w.logger.Warnf("Failed reading file: %s; error: %s", r.fileName, readErr)

			return nil, fmt.Errorf("failed reading wal: %w", readErr)
		}
	}

	w.logger.Debugf("Read %d items", len(items))

	// move to write mode on a new file.
	if err := w.deleteAndCreateFile(); err != nil {
		w.logger.Errorf("Failed to move to a new file: %s", err)

		return nil, err
	}

	w.readMode = false

	w.logger.Infof("Write-Ahead-Log read %d entries, mode: WRITE", len(items))

	return items, nil
}

// truncateAndCloseLogFile when we orderly close a writable log file we truncate it.
// This way, reading it in ReadAll() ends with a io.EOF error after the last record.
func (w *WriteAheadLogFile) truncateAndCloseLogFile() error {
	if w.readMode {
		if err := w.logFile.Close(); err != nil {
			w.logger.Errorf("Failed to close log file: %s; error: %s", w.logFile.Name(), err)

			return err
		}

		w.logger.Debugf("Closed log file: %s", w.logFile.Name())

		return nil
	}

	offset, err := w.logFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	if err = truncateCloseFile(w.logFile, offset); err != nil {
		return err
	}

	w.logger.Debugf("Truncated, Sync'ed & Closed log file: %s", w.logFile.Name())

	return nil
}

func (w *WriteAheadLogFile) switchFiles() error {
	var err error

	w.logger.Debugf("Number of files: %d, active indexes: %v, truncation index: %d",
		len(w.activeIndexes), w.activeIndexes, w.truncateIndex)

	if w.readMode {
		return ErrReadOnly
	}

	if err = w.truncateAndCloseLogFile(); err != nil {
		w.logger.Errorf("Failed to truncateAndCloseLogFile: %s", err)

		return err
	}

	err = w.deleteAndCreateFile()
	if err != nil {
		return err
	}

	w.logger.Debugf("Successfully switched to log file: %s", w.logFile.Name())
	w.logger.Debugf("Number of files: %d, active indexes: %v, truncation index: %d",
		len(w.activeIndexes), w.activeIndexes, w.truncateIndex)

	return nil
}

func (w *WriteAheadLogFile) deleteAndCreateFile() error {
	var err error

	w.index++
	nextFileName := fmt.Sprintf(walFileTemplate, w.index)
	nextFilePath := filepath.Join(w.dirFile.Name(), nextFileName)
	w.logger.Debugf("Preparing next log file: %s", nextFilePath)

	if w.activeIndexes[0] < w.truncateIndex {
		var j int

		for i := 0; w.activeIndexes[i] < w.truncateIndex; i++ {
			deleteFileName := fmt.Sprintf(walFileTemplate, w.activeIndexes[i])
			deleteFilePath := filepath.Join(w.dirFile.Name(), deleteFileName)
			w.logger.Debugf("Delete log file: %s", deleteFileName)

			err = os.Remove(deleteFilePath)
			if err != nil {
				return err
			}

			j = i
		}

		w.activeIndexes = w.activeIndexes[j+1:]
		w.metrics.CountOfFiles.Set(float64(len(w.activeIndexes)))
	}

	w.logger.Debugf("Creating log file: %s", nextFileName)

	w.logFile, err = os.OpenFile(nextFilePath, os.O_CREATE|os.O_WRONLY, walFilePermPrivateRW)
	if err != nil {
		return err
	}

	if _, err = w.logFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if err = w.saveCRC(); err != nil {
		return err
	}

	w.activeIndexes = append(w.activeIndexes, w.index)
	w.metrics.CountOfFiles.Set(float64(len(w.activeIndexes)))

	return nil
}

// saveCRC saves the current CRC followed by a CRC_ANCHOR record.
func (w *WriteAheadLogFile) saveCRC() error {
	anchorRecord := &protos.LogRecord{Type: protos.LogRecord_CRC_ANCHOR, TruncateTo: false}

	b, err := proto.Marshal(anchorRecord)
	if err != nil {
		return err
	}

	recordLength := len(b)

	padSize, padBytes := getPadBytes(recordLength)
	if padSize != 0 {
		b = append(b, padBytes...)
	}

	header := uint64(recordLength) | (uint64(w.crc) << 32)
	binary.LittleEndian.PutUint64(w.headerBuff, header)

	offset, err := w.logFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	nh, err := w.logFile.Write(w.headerBuff)
	if err != nil {
		return fmt.Errorf("wal: failed to write crc-anchor header bytes: %w", err)
	}

	nb, err := w.logFile.Write(b)
	if err != nil {
		return fmt.Errorf("wal: failed to write crc-anchor payload bytes: %w", err)
	}

	err = w.logFile.Sync()
	if err != nil {
		return fmt.Errorf("wal: failed to Sync: %w", err)
	}

	w.logger.Debugf("CRC-Anchor %08X written to file: %s, at offset %d, size=%d", w.crc, w.logFile.Name(), offset, nh+nb)

	return nil
}

func InitializeAndReadAll(
	logger api.Logger,
	walDir string,
	options *Options,
) (writeAheadLog *WriteAheadLogFile, initialState [][]byte, err error) {
	logger.Infof("Trying to creating a Write-Ahead-Log at dir: %s", walDir)
	logger.Debugf("Write-Ahead-Log options: %s", options)

	writeAheadLog, err = Create(logger, walDir, options)
	if err != nil {
		if !errors.Is(err, ErrWALAlreadyExists) {
			err = errors.Wrap(err, "Cannot create Write-Ahead-Log")

			return nil, nil, err
		}

		logger.Infof("Write-Ahead-Log already exists at dir: %s; Trying to open", walDir)

		writeAheadLog, err = Open(logger, walDir, options)
		if err != nil {
			err = errors.Wrap(err, "Cannot open Write-Ahead-Log")

			return nil, nil, err
		}

		initialState, err = writeAheadLog.ReadAll()
		if err != nil {
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				err = errors.Wrap(err, "Cannot read initial state from Write-Ahead-Log")

				return nil, nil, err
			}

			logger.Infof("Received io.ErrUnexpectedEOF, trying to repair Write-Ahead-Log at dir: %s", walDir)

			err = Repair(logger, walDir)
			if err != nil {
				err = errors.Wrap(err, "Cannot repair Write-Ahead-Log")

				return nil, nil, err
			}

			logger.Infof("Reading Write-Ahead-Log initial state after repair")

			initialState, err = writeAheadLog.ReadAll()
			if err != nil {
				err = errors.Wrap(err, "Cannot initial state from Write-Ahead-Log, after repair")

				return nil, nil, err
			}
		}
	}

	logger.Infof("Write-Ahead-Log initialized successfully, initial state contains %d entries", len(initialState))

	return writeAheadLog, initialState, err
}
