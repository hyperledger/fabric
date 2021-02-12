/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mocks

import (
	"errors"
	"hash"
	"reflect"

	"github.com/hyperledger/fabric/bccsp"
)

type Encryptor struct {
	KeyArg       bccsp.Key
	PlaintextArg []byte
	OptsArg      bccsp.EncrypterOpts

	EncValue []byte
	EncErr   error
}

func (e *Encryptor) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	if !reflect.DeepEqual(e.KeyArg, k) {
		return nil, errors.New("invalid key")
	}
	if !reflect.DeepEqual(e.PlaintextArg, plaintext) {
		return nil, errors.New("invalid plaintext")
	}
	if !reflect.DeepEqual(e.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return e.EncValue, e.EncErr
}

type Decryptor struct{}

func (*Decryptor) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	panic("implement me")
}

type Signer struct {
	KeyArg    bccsp.Key
	DigestArg []byte
	OptsArg   bccsp.SignerOpts

	Value []byte
	Err   error
}

func (s *Signer) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	if !reflect.DeepEqual(s.KeyArg, k) {
		return nil, errors.New("invalid key")
	}
	if !reflect.DeepEqual(s.DigestArg, digest) {
		return nil, errors.New("invalid digest")
	}
	if !reflect.DeepEqual(s.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return s.Value, s.Err
}

type Verifier struct {
	KeyArg       bccsp.Key
	SignatureArg []byte
	DigestArg    []byte
	OptsArg      bccsp.SignerOpts

	Value bool
	Err   error
}

func (s *Verifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	if !reflect.DeepEqual(s.KeyArg, k) {
		return false, errors.New("invalid key")
	}
	if !reflect.DeepEqual(s.SignatureArg, signature) {
		return false, errors.New("invalid signature")
	}
	if !reflect.DeepEqual(s.DigestArg, digest) {
		return false, errors.New("invalid digest")
	}
	if !reflect.DeepEqual(s.OptsArg, opts) {
		return false, errors.New("invalid opts")
	}

	return s.Value, s.Err
}

type Hasher struct {
	MsgArg  []byte
	OptsArg bccsp.HashOpts

	Value     []byte
	ValueHash hash.Hash
	Err       error
}

func (h *Hasher) Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error) {
	if !reflect.DeepEqual(h.MsgArg, msg) {
		return nil, errors.New("invalid message")
	}
	if !reflect.DeepEqual(h.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return h.Value, h.Err
}

func (h *Hasher) GetHash(opts bccsp.HashOpts) (hash.Hash, error) {
	if !reflect.DeepEqual(h.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return h.ValueHash, h.Err
}

type KeyGenerator struct {
	OptsArg bccsp.KeyGenOpts

	Value bccsp.Key
	Err   error
}

func (kg *KeyGenerator) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	if !reflect.DeepEqual(kg.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return kg.Value, kg.Err
}

type KeyDeriver struct {
	KeyArg  bccsp.Key
	OptsArg bccsp.KeyDerivOpts

	Value bccsp.Key
	Err   error
}

func (kd *KeyDeriver) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	if !reflect.DeepEqual(kd.KeyArg, k) {
		return nil, errors.New("invalid key")
	}
	if !reflect.DeepEqual(kd.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return kd.Value, kd.Err
}

type KeyImporter struct {
	RawArg  []byte
	OptsArg bccsp.KeyImportOpts

	Value bccsp.Key
	Err   error
}

func (ki *KeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	if !reflect.DeepEqual(ki.RawArg, raw) {
		return nil, errors.New("invalid raw")
	}
	if !reflect.DeepEqual(ki.OptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return ki.Value, ki.Err
}
