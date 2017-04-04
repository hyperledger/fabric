/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"testing"

	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
)

func TestMockBlocksDeliverer(t *testing.T) {
	var bd blocksprovider.BlocksDeliverer
	bd = &MockBlocksDeliverer{}
	_ = bd
}

func TestMockGossipServiceAdapter(t *testing.T) {
	var gsa blocksprovider.GossipServiceAdapter
	gsa = &MockGossipServiceAdapter{}
	_ = gsa

}

func TestMockLedgerInfo(t *testing.T) {
	var li blocksprovider.LedgerInfo
	li = &MockLedgerInfo{}
	_ = li
}
