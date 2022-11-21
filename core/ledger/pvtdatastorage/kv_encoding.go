/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/bits-and-blooms/bitset"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/pkg/errors"
)

var (
	pendingCommitKey                 = []byte{0}
	lastCommittedBlkkey              = []byte{1}
	pvtDataKeyPrefix                 = []byte{2}
	expiryKeyPrefix                  = []byte{3}
	elgPrioritizedMissingDataGroup   = []byte{4}
	inelgMissingDataGroup            = []byte{5}
	collElgKeyPrefix                 = []byte{6}
	lastUpdatedOldBlocksKey          = []byte{7}
	elgDeprioritizedMissingDataGroup = []byte{8}
	bootKVHashesKeyPrefix            = []byte{9}
	lastBlockInBootSnapshotKey       = []byte{'a'}
	hashedIndexKeyPrefix             = []byte{'b'}
	purgeMarkerKeyPrefix             = []byte{'c'}
	purgeMarkerCollKeyPrefix         = []byte{'d'}
	purgeMarkerForReconKeyPrefix     = []byte{'e'}

	nilByte    = byte(0)
	emptyValue = []byte{}
)

func getDataKeysForRangeScanByBlockNum(blockNum uint64) ([]byte, []byte) {
	startKey := append(pvtDataKeyPrefix, version.NewHeight(blockNum, 0).ToBytes()...)
	endKey := append(pvtDataKeyPrefix, version.NewHeight(blockNum+1, 0).ToBytes()...)
	return startKey, endKey
}

func getExpiryKeysForRangeScan(minBlkNum, maxBlkNum uint64) ([]byte, []byte) {
	startKey := append(expiryKeyPrefix, version.NewHeight(minBlkNum, 0).ToBytes()...)
	endKey := append(expiryKeyPrefix, version.NewHeight(maxBlkNum+1, 0).ToBytes()...)
	return startKey, endKey
}

func encodeLastCommittedBlockVal(blockNum uint64) []byte {
	return proto.EncodeVarint(blockNum)
}

func decodeLastCommittedBlockVal(blockNumBytes []byte) uint64 {
	s, _ := proto.DecodeVarint(blockNumBytes)
	return s
}

func encodeDataKey(key *dataKey) []byte {
	dataKeyBytes := append(pvtDataKeyPrefix, version.NewHeight(key.blkNum, key.txNum).ToBytes()...)
	dataKeyBytes = append(dataKeyBytes, []byte(key.ns)...)
	dataKeyBytes = append(dataKeyBytes, nilByte)
	return append(dataKeyBytes, []byte(key.coll)...)
}

func encodeDataValue(collData *rwset.CollectionPvtReadWriteSet) ([]byte, error) {
	return proto.Marshal(collData)
}

func encodeExpiryKey(expiryKey *expiryKey) []byte {
	// reusing version encoding scheme here
	return append(expiryKeyPrefix, version.NewHeight(expiryKey.expiringBlk, expiryKey.committingBlk).ToBytes()...)
}

func encodeExpiryValue(expiryData *ExpiryData) ([]byte, error) {
	return proto.Marshal(expiryData)
}

func decodeExpiryKey(expiryKeyBytes []byte) (*expiryKey, error) {
	height, _, err := version.NewHeightFromBytes(expiryKeyBytes[1:])
	if err != nil {
		return nil, err
	}
	return &expiryKey{expiringBlk: height.BlockNum, committingBlk: height.TxNum}, nil
}

func decodeExpiryValue(expiryValueBytes []byte) (*ExpiryData, error) {
	expiryData := &ExpiryData{}
	err := proto.Unmarshal(expiryValueBytes, expiryData)
	return expiryData, errors.Wrap(err, "error while decoding expiry value")
}

func decodeDatakey(datakeyBytes []byte) (*dataKey, error) {
	v, n, err := version.NewHeightFromBytes(datakeyBytes[1:])
	if err != nil {
		return nil, err
	}
	blkNum := v.BlockNum
	tranNum := v.TxNum
	remainingBytes := datakeyBytes[n+1:]
	nilByteIndex := bytes.IndexByte(remainingBytes, nilByte)
	ns := string(remainingBytes[:nilByteIndex])
	coll := string(remainingBytes[nilByteIndex+1:])
	return &dataKey{nsCollBlk{ns, coll, blkNum}, tranNum}, nil
}

func decodeDataValue(datavalueBytes []byte) (*rwset.CollectionPvtReadWriteSet, error) {
	collPvtdata := &rwset.CollectionPvtReadWriteSet{}
	err := proto.Unmarshal(datavalueBytes, collPvtdata)
	return collPvtdata, err
}

func encodeElgPrioMissingDataKey(key *missingDataKey) []byte {
	// When missing pvtData reconciler asks for missing data info,
	// it is necessary to pass the missing pvtdata info associated with
	// the most recent block so that missing pvtdata in the state db can
	// be fixed sooner to reduce the "private data matching public hash version
	// is not available" error during endorserments. In order to give priority
	// to missing pvtData in the most recent block, we use reverse order
	// preserving encoding for the missing data key. This simplifies the
	// implementation of GetMissingPvtDataInfoForMostRecentBlocks().
	encKey := append(elgPrioritizedMissingDataGroup, encodeReverseOrderVarUint64(key.blkNum)...)
	encKey = append(encKey, []byte(key.ns)...)
	encKey = append(encKey, nilByte)
	return append(encKey, []byte(key.coll)...)
}

func encodeElgDeprioMissingDataKey(key *missingDataKey) []byte {
	encKey := append(elgDeprioritizedMissingDataGroup, encodeReverseOrderVarUint64(key.blkNum)...)
	encKey = append(encKey, []byte(key.ns)...)
	encKey = append(encKey, nilByte)
	return append(encKey, []byte(key.coll)...)
}

func decodeElgMissingDataKey(keyBytes []byte) *missingDataKey {
	key := &missingDataKey{nsCollBlk: nsCollBlk{}}
	blkNum, numBytesConsumed := decodeReverseOrderVarUint64(keyBytes[1:])
	splittedKey := bytes.Split(keyBytes[numBytesConsumed+1:], []byte{nilByte})
	key.ns = string(splittedKey[0])
	key.coll = string(splittedKey[1])
	key.blkNum = blkNum
	return key
}

func encodeInelgMissingDataKey(key *missingDataKey) []byte {
	encKey := append(inelgMissingDataGroup, []byte(key.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(key.coll)...)
	encKey = append(encKey, nilByte)
	return append(encKey, []byte(encodeReverseOrderVarUint64(key.blkNum))...)
}

func decodeInelgMissingDataKey(keyBytes []byte) *missingDataKey {
	key := &missingDataKey{nsCollBlk: nsCollBlk{}}
	splittedKey := bytes.SplitN(keyBytes[1:], []byte{nilByte}, 3) // encoded bytes for blknum may contain empty bytes
	key.ns = string(splittedKey[0])
	key.coll = string(splittedKey[1])
	key.blkNum, _ = decodeReverseOrderVarUint64(splittedKey[2])
	return key
}

func encodeMissingDataValue(bitmap *bitset.BitSet) ([]byte, error) {
	return bitmap.MarshalBinary()
}

func decodeMissingDataValue(bitmapBytes []byte) (*bitset.BitSet, error) {
	bitmap := &bitset.BitSet{}
	if err := bitmap.UnmarshalBinary(bitmapBytes); err != nil {
		return nil, errors.Wrap(err, "error while decoding missing data value")
	}
	return bitmap, nil
}

func encodeCollElgKey(blkNum uint64) []byte {
	return append(collElgKeyPrefix, encodeReverseOrderVarUint64(blkNum)...)
}

func decodeCollElgKey(b []byte) uint64 {
	blkNum, _ := decodeReverseOrderVarUint64(b[1:])
	return blkNum
}

func encodeCollElgVal(m *CollElgInfo) ([]byte, error) {
	return proto.Marshal(m)
}

func decodeCollElgVal(b []byte) (*CollElgInfo, error) {
	m := &CollElgInfo{}
	if err := proto.Unmarshal(b, m); err != nil {
		return nil, errors.WithStack(err)
	}
	return m, nil
}

func encodeBootKVHashesKey(key *bootKVHashesKey) []byte {
	k := append(bootKVHashesKeyPrefix, version.NewHeight(key.blkNum, key.txNum).ToBytes()...)
	k = append(k, []byte(key.ns)...)
	k = append(k, nilByte)
	return append(k, []byte(key.coll)...)
}

func encodeBootKVHashesVal(val *BootKVHashes) ([]byte, error) {
	b, err := proto.Marshal(val)
	if err != nil {
		return nil, errors.Wrap(err, "error while marshalling BootKVHashes")
	}
	return b, nil
}

func decodeBootKVHashesVal(b []byte) (*BootKVHashes, error) {
	val := &BootKVHashes{}
	if err := proto.Unmarshal(b, val); err != nil {
		return nil, errors.Wrap(err, "error while unmarshalling bytes for BootKVHashes")
	}
	return val, nil
}

func encodeLastBlockInBootSnapshotVal(blockNum uint64) []byte {
	return proto.EncodeVarint(blockNum)
}

func decodeLastBlockInBootSnapshotVal(blockNumBytes []byte) (uint64, error) {
	s, n := proto.DecodeVarint(blockNumBytes)
	if n == 0 {
		return 0, errors.New("unexpected bytes for interpreting as varint")
	}
	return s, nil
}

func createRangeScanKeysForElgMissingData(blkNum uint64, group []byte) ([]byte, []byte) {
	startKey := append(group, encodeReverseOrderVarUint64(blkNum)...)
	endKey := append(group, encodeReverseOrderVarUint64(0)...)

	return startKey, endKey
}

func createRangeScanKeysForInelgMissingData(maxBlkNum uint64, ns, coll string) ([]byte, []byte) {
	startKey := encodeInelgMissingDataKey(
		&missingDataKey{
			nsCollBlk: nsCollBlk{
				ns:     ns,
				coll:   coll,
				blkNum: maxBlkNum,
			},
		},
	)
	endKey := encodeInelgMissingDataKey(
		&missingDataKey{
			nsCollBlk: nsCollBlk{
				ns:     ns,
				coll:   coll,
				blkNum: 0,
			},
		},
	)

	return startKey, endKey
}

func createRangeScanKeysForCollElg() (startKey, endKey []byte) {
	return encodeCollElgKey(math.MaxUint64),
		encodeCollElgKey(0)
}

func entireDatakeyRange() ([]byte, []byte) {
	startKey := append(pvtDataKeyPrefix, version.NewHeight(0, 0).ToBytes()...)
	endKey := append(pvtDataKeyPrefix, version.NewHeight(math.MaxUint64, math.MaxUint64).ToBytes()...)
	return startKey, endKey
}

func eligibleMissingdatakeyRange(blkNum uint64) ([]byte, []byte) {
	startKey := append(elgPrioritizedMissingDataGroup, encodeReverseOrderVarUint64(blkNum)...)
	endKey := append(elgPrioritizedMissingDataGroup, encodeReverseOrderVarUint64(blkNum-1)...)
	return startKey, endKey
}

func encodeHashedIndexKey(k *hashedIndexKey) []byte {
	encKey := append(hashedIndexKeyPrefix, []byte(k.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(k.coll)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, k.pvtkeyHash...)
	return append(encKey, version.NewHeight(k.blkNum, k.txNum).ToBytes()...)
}

func encodePurgeMarkerCollKey(k *purgeMarkerCollKey) []byte {
	encKey := append(purgeMarkerCollKeyPrefix, []byte(k.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(k.coll)...)
	return encKey
}

func encodePurgeMarkerKey(k *purgeMarkerKey) []byte {
	encKey := append(purgeMarkerKeyPrefix, []byte(k.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(k.coll)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, k.pvtkeyHash...)
	return encKey
}

func encodePurgeMarkerForReconKey(k *purgeMarkerKey) []byte {
	encKey := append(purgeMarkerForReconKeyPrefix, []byte(k.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(k.coll)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, k.pvtkeyHash...)
	return encKey
}

func rangeScanKeysForPurgeMarkers() ([]byte, []byte) {
	return purgeMarkerKeyPrefix, []byte{purgeMarkerKeyPrefix[0] + 1}
}

// driveHashedIndexKeyRangeFromPurgeMarker returns the scan range for hashedIndexKeys for a key specified by the `purgeMarkerKey`.
// The range covers all the hashedIndexKeys between block 0 and the height specified in the `purgeMarkerVal`
func driveHashedIndexKeyRangeFromPurgeMarker(purgeMarkerKey, purgeMarkerVal []byte) ([]byte, []byte, error) {
	startKey := append(hashedIndexKeyPrefix, purgeMarkerKey[1:]...)
	version, err := decodePurgeMarkerVal(purgeMarkerVal)
	if err != nil {
		return nil, nil, err
	}
	version.TxNum += 1 // increase transaction by one so that the private key for the purge operation itself is also included
	endKey := append(startKey, version.ToBytes()...)
	return startKey, endKey, nil
}

func rangeScanKeysForHashedIndexKey(ns, coll string, keyHash []byte) ([]byte, []byte) {
	startKey := encodeHashedIndexKey(
		&hashedIndexKey{
			ns:         ns,
			coll:       coll,
			pvtkeyHash: keyHash,
		},
	)
	endKey := encodeHashedIndexKey(
		&hashedIndexKey{
			ns:         ns,
			coll:       coll,
			pvtkeyHash: keyHash,
			blkNum:     math.MaxUint64,
			txNum:      math.MaxUint64,
		},
	)
	return startKey, endKey
}

func encodePurgeMarkerVal(v *purgeMarkerVal) []byte {
	return version.NewHeight(v.blkNum, v.txNum).ToBytes()
}

func decodePurgeMarkerVal(b []byte) (*version.Height, error) {
	v, _, err := version.NewHeightFromBytes(b)
	return v, err
}

func deriveDataKeyFromEncodedHashedIndexKey(encHashedIndexKey []byte) ([]byte, error) {
	firstNilByteIndex := 0
	secondNilByteIndex := 0
	foundFirstNilByte := false
	lengthHashedPvtKey := 32 // 256/8 bytes

	for i, b := range encHashedIndexKey {
		if b == 0x00 {
			if !foundFirstNilByte {
				firstNilByteIndex = i
				foundFirstNilByte = true
			} else {
				secondNilByteIndex = i
				break
			}
		}
	}

	if secondNilByteIndex == 0 {
		return nil, errors.Errorf("unexpected bytes [%x] for HashedIndexed key", encHashedIndexKey)
	}

	ns := encHashedIndexKey[1:firstNilByteIndex]
	coll := encHashedIndexKey[firstNilByteIndex+1 : secondNilByteIndex]
	blkNumTxNumBytes := encHashedIndexKey[secondNilByteIndex+lengthHashedPvtKey+1:]

	encDataKey := append(pvtDataKeyPrefix, blkNumTxNumBytes...)
	encDataKey = append(encDataKey, ns...)
	encDataKey = append(encDataKey, nilByte)
	encDataKey = append(encDataKey, coll...)
	return encDataKey, nil
}

// encodeReverseOrderVarUint64 returns a byte-representation for a uint64 number such that
// the number is first subtracted from MaxUint64 and then all the leading 0xff bytes
// are trimmed and replaced by the number of such trimmed bytes. This helps in reducing the size.
// In the byte order comparison this encoding ensures that EncodeReverseOrderVarUint64(A) > EncodeReverseOrderVarUint64(B),
// If B > A
func encodeReverseOrderVarUint64(number uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, math.MaxUint64-number)
	numFFBytes := 0
	for _, b := range bytes {
		if b != 0xff {
			break
		}
		numFFBytes++
	}
	size := 8 - numFFBytes
	encodedBytes := make([]byte, size+1)
	encodedBytes[0] = proto.EncodeVarint(uint64(numFFBytes))[0]
	copy(encodedBytes[1:], bytes[numFFBytes:])
	return encodedBytes
}

// decodeReverseOrderVarUint64 decodes the number from the bytes obtained from function 'EncodeReverseOrderVarUint64'.
// Also, returns the number of bytes that are consumed in the process
func decodeReverseOrderVarUint64(bytes []byte) (uint64, int) {
	s, _ := proto.DecodeVarint(bytes)
	numFFBytes := int(s)
	decodedBytes := make([]byte, 8)
	realBytesNum := 8 - numFFBytes
	copy(decodedBytes[numFFBytes:], bytes[1:realBytesNum+1])
	numBytesConsumed := realBytesNum + 1
	for i := 0; i < numFFBytes; i++ {
		decodedBytes[i] = 0xff
	}
	return (math.MaxUint64 - binary.BigEndian.Uint64(decodedBytes)), numBytesConsumed
}
