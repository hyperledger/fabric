package bdls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"


	"github.com/BDLS-bft/bdls/crypto/blake2b"
	"github.com/BDLS-bft/bdls/crypto/btcec"

	proto "github.com/gogo/protobuf/proto"
)

// ErrPubKey will be returned if error found while decoding message's public key
var ErrPubKey = errors.New("incorrect pubkey format")

// secp256k1 elliptic curve
var S256Curve elliptic.Curve = btcec.S256()

const (
	// SizeAxis defines byte size of X-axis or Y-axis in a public key
	SizeAxis = 32
	// SignaturePrefix is the prefix for signing a consensus message
	SignaturePrefix = "BDLS_CONSENSUS_SIGNATURE"
)

// PubKeyAxis defines X-axis or Y-axis in a public key
type PubKeyAxis [SizeAxis]byte

// Marshal implements protobuf MarshalTo
func (t PubKeyAxis) Marshal() ([]byte, error) {
	return t[:], nil
}

// MarshalTo implements protobuf MarshalTo
func (t *PubKeyAxis) MarshalTo(data []byte) (n int, err error) {
	copy(data, (*t)[:])
	return SizeAxis, nil
}

// Unmarshal implements protobuf Unmarshal
func (t *PubKeyAxis) Unmarshal(data []byte) error {
	// more than 32 bytes, illegal axis
	if len(data) > SizeAxis {
		return ErrPubKey
	}

	// if data is less than 32 bytes, we MUST keep the leading 0 zeros.
	off := SizeAxis - len(data)
	copy((*t)[off:], data)
	return nil
}

// Size implements protobuf Size
func (t *PubKeyAxis) Size() int { return SizeAxis }

// String representation of Axis
func (t *PubKeyAxis) String() string {
	return hex.EncodeToString((*t)[:])
}

// String representation of Axis
func (t *PubKeyAxis) MarshalText() (text []byte, err error) {
	return []byte(hex.EncodeToString((*t)[:])), nil
}

// Identity is a user-defined struct to encode X-axis and Y-axis for a publickey in an array
type Identity [2 * SizeAxis]byte

// default method to derive coordinate from public key
func DefaultPubKeyToIdentity(pubkey *ecdsa.PublicKey) (ret Identity) {
	var X PubKeyAxis
	var Y PubKeyAxis

	err := X.Unmarshal(pubkey.X.Bytes())
	if err != nil {
		panic(err)
	}

	err = Y.Unmarshal(pubkey.Y.Bytes())
	if err != nil {
		panic(err)
	}

	copy(ret[:SizeAxis], X[:])
	copy(ret[SizeAxis:], Y[:])
	return
}

// Hash concats and hash as follows:
// blake2b(signPrefix + version + pubkey.X + pubkey.Y+len_32bit(msg) + message)
func (sp *SignedProto) Hash() []byte {
	hash, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}
	// write prefix
	_, err = hash.Write([]byte(SignaturePrefix))
	if err != nil {
		panic(err)
	}

	// write version
	err = binary.Write(hash, binary.LittleEndian, sp.Version)
	if err != nil {
		panic(err)
	}

	// write X & Y
	_, err = hash.Write(sp.X[:])
	if err != nil {
		panic(err)
	}

	_, err = hash.Write(sp.Y[:])
	if err != nil {
		panic(err)
	}

	// write message length
	err = binary.Write(hash, binary.LittleEndian, uint32(len(sp.Message)))
	if err != nil {
		panic(err)
	}

	// write message
	_, err = hash.Write(sp.Message)
	if err != nil {
		panic(err)
	}

	return hash.Sum(nil)
}

// Sign the message with a private key
func (sp *SignedProto) Sign(m *Message, privateKey *ecdsa.PrivateKey) {
	bts, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	// hash message
	sp.Version = ProtocolVersion
	sp.Message = bts

	err = sp.X.Unmarshal(privateKey.PublicKey.X.Bytes())
	if err != nil {
		panic(err)
	}
	err = sp.Y.Unmarshal(privateKey.PublicKey.Y.Bytes())
	if err != nil {
		panic(err)
	}
	hash := sp.Hash()

	// sign the message
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash)
	if err != nil {
		panic(err)
	}
	sp.R = r.Bytes()
	sp.S = s.Bytes()
}

// Verify the signature of this signed message
func (sp *SignedProto) Verify(curve elliptic.Curve) bool {
	var X, Y, R, S big.Int
	hash := sp.Hash()
	// verify against public key and r, s
	pubkey := ecdsa.PublicKey{}
	pubkey.Curve = curve
	pubkey.X = &X
	pubkey.Y = &Y
	X.SetBytes(sp.X[:])
	Y.SetBytes(sp.Y[:])
	R.SetBytes(sp.R[:])
	S.SetBytes(sp.S[:])

	return ecdsa.Verify(&pubkey, hash, &R, &S)
}

// PublicKey returns the public key of this signed message
func (sp *SignedProto) PublicKey(curve elliptic.Curve) *ecdsa.PublicKey {
	pubkey := new(ecdsa.PublicKey)
	pubkey.Curve = curve
	pubkey.X = big.NewInt(0).SetBytes(sp.X[:])
	pubkey.Y = big.NewInt(0).SetBytes(sp.Y[:])
	return pubkey
}
