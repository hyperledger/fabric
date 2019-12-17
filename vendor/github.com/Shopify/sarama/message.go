package sarama

import (
	"fmt"
	"time"
)

// CompressionCodec represents the various compression codecs recognized by Kafka in messages.
type CompressionCodec int8

// The lowest 3 bits contain the compression codec used for the message
const compressionCodecMask int8 = 0x07

const (
	CompressionNone   CompressionCodec = 0
	CompressionGZIP   CompressionCodec = 1
	CompressionSnappy CompressionCodec = 2
	CompressionLZ4    CompressionCodec = 3
	CompressionZSTD   CompressionCodec = 4
)

func (cc CompressionCodec) String() string {
	return []string{
		"none",
		"gzip",
		"snappy",
		"lz4",
	}[int(cc)]
}

// CompressionLevelDefault is the constant to use in CompressionLevel
// to have the default compression level for any codec. The value is picked
// that we don't use any existing compression levels.
const CompressionLevelDefault = -1000

type Message struct {
	Codec            CompressionCodec // codec used to compress the message contents
	CompressionLevel int              // compression level
	Key              []byte           // the message key, may be nil
	Value            []byte           // the message contents
	Set              *MessageSet      // the message set a message might wrap
	Version          int8             // v1 requires Kafka 0.10
	Timestamp        time.Time        // the timestamp of the message (version 1+ only)

	compressedCache []byte
	compressedSize  int // used for computing the compression ratio metrics
}

func (m *Message) encode(pe packetEncoder) error {
	pe.push(newCRC32Field(crcIEEE))

	pe.putInt8(m.Version)

	attributes := int8(m.Codec) & compressionCodecMask
	pe.putInt8(attributes)

	if m.Version >= 1 {
		if err := (Timestamp{&m.Timestamp}).encode(pe); err != nil {
			return err
		}
	}

	err := pe.putBytes(m.Key)
	if err != nil {
		return err
	}

	var payload []byte

	if m.compressedCache != nil {
		payload = m.compressedCache
		m.compressedCache = nil
	} else if m.Value != nil {

		payload, err = compress(m.Codec, m.CompressionLevel, m.Value)
		if err != nil {
			return err
		}
		m.compressedCache = payload
		// Keep in mind the compressed payload size for metric gathering
		m.compressedSize = len(payload)
	}

	if err = pe.putBytes(payload); err != nil {
		return err
	}

	return pe.pop()
}

func (m *Message) decode(pd packetDecoder) (err error) {
	err = pd.push(newCRC32Field(crcIEEE))
	if err != nil {
		return err
	}

	m.Version, err = pd.getInt8()
	if err != nil {
		return err
	}

	if m.Version > 1 {
		return PacketDecodingError{fmt.Sprintf("unknown magic byte (%v)", m.Version)}
	}

	attribute, err := pd.getInt8()
	if err != nil {
		return err
	}
	m.Codec = CompressionCodec(attribute & compressionCodecMask)

	if m.Version == 1 {
		if err := (Timestamp{&m.Timestamp}).decode(pd); err != nil {
			return err
		}
	}

	m.Key, err = pd.getBytes()
	if err != nil {
		return err
	}

	m.Value, err = pd.getBytes()
	if err != nil {
		return err
	}

	// Required for deep equal assertion during tests but might be useful
	// for future metrics about the compression ratio in fetch requests
	m.compressedSize = len(m.Value)

	switch m.Codec {
	case CompressionNone:
		// nothing to do
	default:
		if m.Value == nil {
			break
		}

		m.Value, err = decompress(m.Codec, m.Value)
		if err != nil {
			return err
		}
		if err := m.decodeSet(); err != nil {
			return err
		}
	}

	return pd.pop()
}

// decodes a message set from a previousy encoded bulk-message
func (m *Message) decodeSet() (err error) {
	pd := realDecoder{raw: m.Value}
	m.Set = &MessageSet{}
	return m.Set.decode(&pd)
}
