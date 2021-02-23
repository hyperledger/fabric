/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type NewHashFunc func() (hash.Hash, error)

// FileWriter creates a new file for ledger snapshot. This is expected to be used by various
// components of ledger, such as blockstorage and statedb for exporting the relevant snapshot data
type FileWriter struct {
	file              *os.File
	hasher            hash.Hash
	bufWriter         *bufio.Writer
	multiWriter       io.Writer
	varintReusableBuf []byte
}

// CreateFile creates a new file for exporting the ledger snapshot data
// This function returns an error if the file already exists. The `dataformat` is the first byte
// written to the file. The function newHash is used to construct an hash.Hash for computing the hash-sum of the data stream
func CreateFile(filePath string, dataformat byte, newHashFunc NewHashFunc) (*FileWriter, error) {
	hashImpl, err := newHashFunc()
	if err != nil {
		return nil, err
	}
	// create the file only if it does not already exist.
	// set the permission mode to read-only, as once the file is closed, we do not support modifying the file
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o444)
	if err != nil {
		return nil, errors.Wrapf(err, "error while creating the snapshot file: %s", filePath)
	}
	bufWriter := bufio.NewWriter(file)
	multiWriter := io.MultiWriter(bufWriter, hashImpl)
	if _, err := multiWriter.Write([]byte{dataformat}); err != nil {
		file.Close()
		return nil, errors.Wrapf(err, "error while writing data format to the snapshot file: %s", filePath)
	}
	return &FileWriter{
		file:              file,
		bufWriter:         bufWriter,
		multiWriter:       multiWriter,
		hasher:            hashImpl,
		varintReusableBuf: make([]byte, binary.MaxVarintLen64),
	}, nil
}

// EncodeString encodes and appends the string to the data stream
func (c *FileWriter) EncodeString(str string) error {
	return c.EncodeBytes([]byte(str))
}

// EncodeString encodes and appends a proto message to the data stream
func (c *FileWriter) EncodeProtoMessage(m proto.Message) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return errors.Wrapf(err, "error marshalling proto message to write to the snapshot file: %s", c.file.Name())
	}
	return c.EncodeBytes(b)
}

// EncodeBytes encodes and appends bytes to the data stream
func (c *FileWriter) EncodeBytes(b []byte) error {
	if err := c.EncodeUVarint(uint64(len(b))); err != nil {
		return err
	}
	if _, err := c.multiWriter.Write(b); err != nil {
		return errors.Wrapf(err, "error while writing data to the snapshot file: %s", c.file.Name())
	}
	return nil
}

// EncodeUVarint encodes and appends a number to the data stream
func (c *FileWriter) EncodeUVarint(u uint64) error {
	n := binary.PutUvarint(c.varintReusableBuf, u)
	if _, err := c.multiWriter.Write(c.varintReusableBuf[:n]); err != nil {
		return errors.Wrapf(err, "error while writing data to the snapshot file: %s", c.file.Name())
	}
	return nil
}

// Done closes the snapshot file and returns the final hash of the data stream
func (c *FileWriter) Done() ([]byte, error) {
	if err := c.bufWriter.Flush(); err != nil {
		return nil, errors.Wrapf(err, "error while flushing to the snapshot file: %s ", c.file.Name())
	}
	if err := c.file.Sync(); err != nil {
		return nil, err
	}
	if err := c.file.Close(); err != nil {
		return nil, errors.Wrapf(err, "error while closing the snapshot file: %s ", c.file.Name())
	}
	return c.hasher.Sum(nil), nil
}

// Close closes the underlying file, if not already done. A consumer can invoke this function if the consumer
// encountered some error and simply wants to abandon the snapshot file creation (typically, intended to be used in a defer statement)
func (c *FileWriter) Close() error {
	if c == nil {
		return nil
	}
	return errors.Wrapf(c.file.Close(), "error while closing the snapshot file: %s", c.file.Name())
}

// FileReader reads from a ledger snapshot file. This is expected to be used for loading the ledger snapshot data
// during bootstrapping a channel from snapshot. The data should be read, using the functions `DecodeXXX`,
// in the same sequence in which the data was written by the functions `EncodeXXX` in the `FileCreator`.
// Note that the FileReader does not verify the hash of stream and it is expected that the hash has been verified
// by the consumer. Later, if we decide to perform this, on-the-side, while loading the snapshot data, the FileRedear,
// like the FileCreator, would take a `hasher` as an input
type FileReader struct {
	file              *os.File
	bufReader         *bufio.Reader
	reusableByteSlice []byte
}

// OpenFile constructs a FileReader. This function returns an error if the format of the file, stored in the
// first byte, does not match with the expectedDataFormat
func OpenFile(filePath string, expectDataformat byte) (*FileReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "error while opening the snapshot file: %s", filePath)
	}
	bufReader := bufio.NewReader(file)
	dataFormat, err := bufReader.ReadByte()
	if err != nil {
		file.Close()
		return nil, errors.Wrapf(err, "error while reading from the snapshot file: %s", filePath)
	}
	if dataFormat != expectDataformat {
		file.Close()
		return nil, errors.New(fmt.Sprintf("unexpected data format: %x", dataFormat))
	}
	return &FileReader{
		file:      file,
		bufReader: bufReader,
	}, nil
}

// DecodeString reads and decodes a string
func (r *FileReader) DecodeString() (string, error) {
	b, err := r.decodeBytes()
	return string(b), err
}

// DecodeBytes reads and decodes bytes
func (r *FileReader) DecodeBytes() ([]byte, error) {
	b, err := r.decodeBytes()
	if err != nil {
		return nil, err
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c, nil
}

// DecodeUVarInt reads and decodes a number
func (r *FileReader) DecodeUVarInt() (uint64, error) {
	u, err := binary.ReadUvarint(r.bufReader)
	if err != nil {
		return 0, errors.Wrapf(err, "error while reading from snapshot file: %s", r.file.Name())
	}
	return u, nil
}

// DecodeProtoMessage reads and decodes a protoMessage
func (r *FileReader) DecodeProtoMessage(m proto.Message) error {
	b, err := r.decodeBytes()
	if err != nil {
		return err
	}
	return proto.Unmarshal(b, m)
}

// Close closes the file
func (r *FileReader) Close() error {
	if r == nil {
		return nil
	}
	return errors.Wrapf(r.file.Close(), "error while closing the snapshot file: %s", r.file.Name())
}

func (r *FileReader) decodeBytes() ([]byte, error) {
	sizeUint, err := r.DecodeUVarInt()
	if err != nil {
		return nil, err
	}
	size := int(sizeUint)
	if size == 0 {
		return []byte{}, nil
	}
	if len(r.reusableByteSlice) < size {
		r.reusableByteSlice = make([]byte, size)
	}
	if _, err := io.ReadFull(r.bufReader, r.reusableByteSlice[0:size]); err != nil {
		return nil, errors.Wrapf(err, "error while reading from snapshot file: %s", r.file.Name())
	}
	return r.reusableByteSlice[0:size], nil
}
