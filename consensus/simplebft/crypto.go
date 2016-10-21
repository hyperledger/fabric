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

package simplebft

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/golang/protobuf/proto"
)

func hash2str(h []byte) string {
	return base64.RawStdEncoding.EncodeToString(h)
}

func hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func merkleHashData(data [][]byte) []byte {
	var digests [][]byte
	for _, d := range data {
		digests = append(digests, hash(d))
	}
	return merkleHashDigests(digests)
}

func merkleHashDigests(digests [][]byte) []byte {
	for len(digests) > 1 {
		var nextDigests [][]byte
		var prev []byte
		for _, d := range digests {
			if prev == nil {
				prev = d
			} else {
				h := sha256.New()
				h.Write(prev)
				h.Write(d)
				nextDigests = append(nextDigests, h.Sum(nil))
				prev = nil
			}
		}
		if prev != nil {
			nextDigests = append(nextDigests, prev)
		}
		digests = nextDigests
	}

	if len(digests) == 0 {
		return nil
	}
	return digests[0]
}

////////////////////////////////////////////////

func (s *SBFT) sign(msg proto.Message) *Signed {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	sig := s.sys.Sign(bytes)
	return &Signed{Data: bytes, Signature: []byte(sig)}
}

func (s *SBFT) checkSig(sig *Signed, signer uint64, msg proto.Message) error {
	err := s.checkBytesSig(sig.Data, signer, sig.Signature)
	if err != nil {
		return err
	}
	err = proto.Unmarshal(sig.Data, msg)
	if err != nil {
		return err
	}
	return nil
}

func (s *SBFT) checkBytesSig(digest []byte, signer uint64, sig []byte) error {
	return s.sys.CheckSig(digest, signer, sig)
}
