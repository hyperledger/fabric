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

package version

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
)

func TestVersionSerialization(t *testing.T) {
	h1 := NewHeight(10, 100)
	b := h1.ToBytes()
	h2, n := NewHeightFromBytes(b)
	testutil.AssertEquals(t, h2, h1)
	testutil.AssertEquals(t, n, len(b))
}

func TestVersionComparison(t *testing.T) {
	testutil.AssertEquals(t, NewHeight(10, 100).Compare(NewHeight(9, 1000)), 1)
	testutil.AssertEquals(t, NewHeight(10, 100).Compare(NewHeight(10, 90)), 1)
	testutil.AssertEquals(t, NewHeight(10, 100).Compare(NewHeight(11, 1)), -1)
	testutil.AssertEquals(t, NewHeight(10, 100).Compare(NewHeight(10, 100)), 0)

	testutil.AssertEquals(t, AreSame(NewHeight(10, 100), NewHeight(10, 100)), true)
	testutil.AssertEquals(t, AreSame(nil, nil), true)
	testutil.AssertEquals(t, AreSame(NewHeight(10, 100), nil), false)
}

func TestVersionExtraBytes(t *testing.T) {
	extraBytes := []byte("junk")
	h1 := NewHeight(10, 100)
	b := h1.ToBytes()
	b1 := append(b, extraBytes...)
	h2, n := NewHeightFromBytes(b1)
	testutil.AssertEquals(t, h2, h1)
	testutil.AssertEquals(t, n, len(b))
	testutil.AssertEquals(t, b1[n:], extraBytes)
}
