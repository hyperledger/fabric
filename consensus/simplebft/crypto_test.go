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
	"encoding/base64"
	"testing"
)

func TestMerkleHash(t *testing.T) {
	data := [][]byte{[]byte("A"), []byte("B"), []byte("C")}
	digest := "2+EeNqqJqWMQPef4rQnBEAwGzNXFrUJMp0HvsGidxCc="
	h := merkleHashData(data)
	hs := base64.StdEncoding.EncodeToString(h)
	if hs != digest {
		t.Errorf("wrong digest, expected %s, got %s", digest, hs)
	}
}
