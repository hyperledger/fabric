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

package java

import (
	"testing"

	"encoding/hex"

	"github.com/hyperledger/fabric/core/util"

	"bytes"
	"os"
)

func TestHashDiffRemoteRepo(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)
	// TODO Change reference to an official test repo once established
	// Same repo being used in behave tests

	srcPath1, err := getCodeFromHTTP("https://github.com/xspeedcruiser/javachaincodemvn")
	srcPath2, err := getCodeFromHTTP("https://github.com/xspeedcruiser/javachaincode")

	if err != nil {
		t.Logf("Error getting code from remote repo %s", err)
		t.Fail()
	}

	defer func() {
		os.RemoveAll(srcPath1)
	}()
	defer func() {
		os.RemoveAll(srcPath2)
	}()
	hash1, err := hashFilesInDir(srcPath1, srcPath1, hash, nil)
	if err != nil {
		t.Logf("Error getting code %s", err)
		t.Fail()
	}
	hash2, err := hashFilesInDir(srcPath2, srcPath2, hash, nil)
	if err != nil {
		t.Logf("Error getting code %s", err)
		t.Fail()
	}
	if bytes.Compare(hash1, hash2) == 0 {
		t.Logf("Hash should be different for 2 different remote repos")
		t.Fail()
	}

}
func TestHashSameRemoteRepo(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

	srcPath1, err := getCodeFromHTTP("https://github.com/xspeedcruiser/javachaincodemvn")
	srcPath2, err := getCodeFromHTTP("https://github.com/xspeedcruiser/javachaincodemvn")

	if err != nil {
		t.Logf("Error getting code from remote repo %s", err)
		t.Fail()
	}

	defer func() {
		os.RemoveAll(srcPath1)
	}()
	defer func() {
		os.RemoveAll(srcPath2)
	}()
	hash1, err := hashFilesInDir(srcPath1, srcPath1, hash, nil)
	if err != nil {
		t.Logf("Error getting code %s", err)
		t.Fail()
	}
	hash2, err := hashFilesInDir(srcPath2, srcPath2, hash, nil)
	if err != nil {
		t.Logf("Error getting code %s", err)
		t.Fail()
	}
	if bytes.Compare(hash1, hash2) != 0 {
		t.Logf("Hash should be same across multiple downloads")
		t.Fail()
	}
}

func TestHashOverLocalDir(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

	hash, err := hashFilesInDir(".", "../golang/hashtestfiles", hash, nil)

	if err != nil {
		t.Fail()
		t.Logf("error : %s", err)
	}

	expectedHash := "7b3b2193bed2bd7c19300aa5d6d7f6bb4d61602e4978a78bc08028379cb5cf0ed877bd9db3e990230e8bf6c974edd765f3027f061fd8657d30fc858a676a6f4a"

	computedHash := hex.EncodeToString(hash[:])

	if expectedHash != computedHash {
		t.Fail()
		t.Logf("Hash expected to be unchanged")
	}
}
