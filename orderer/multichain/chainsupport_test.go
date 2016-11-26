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

package multichain

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
)

func TestCommitConfig(t *testing.T) {
	mcm := &mockConfigtxManager{}
	ctx1 := makeConfigTx("foo", 0)
	wi := newWriteInterceptor(mcm, &mockLedgerWriter{})
	wi.Append([]*cb.Envelope{ctx1}, nil)
	if mcm.config == nil {
		t.Fatalf("Should have applied configuration")
	}
}

func TestIgnoreMultiConfig(t *testing.T) {
	mcm := &mockConfigtxManager{}
	ctx1 := makeConfigTx("foo", 0)
	ctx2 := makeConfigTx("foo", 1)
	wi := newWriteInterceptor(mcm, &mockLedgerWriter{})
	wi.Append([]*cb.Envelope{ctx1, ctx2}, nil)
	if mcm.config != nil {
		t.Fatalf("Should not have applied configuration, we should only check batches with a single tx")
	}
}

func TestIgnoreSingleNonConfig(t *testing.T) {
	mcm := &mockConfigtxManager{}
	ctx1 := makeNormalTx("foo", 0)
	wi := newWriteInterceptor(mcm, &mockLedgerWriter{})
	wi.Append([]*cb.Envelope{ctx1}, nil)
	if mcm.config != nil {
		t.Fatalf("Should not have applied configuration, it was a normal transaction")
	}

}
