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

package kafka

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
)

func TestChainPartition(t *testing.T) {
	cp := newChainPartition(provisional.TestChainID, rawPartition)

	expectedTopic := fmt.Sprintf("%x", provisional.TestChainID)
	actualTopic := cp.Topic()
	if strings.Compare(expectedTopic, actualTopic) != 0 {
		t.Fatalf("Got the wrong topic, expected %s, got %s instead", expectedTopic, actualTopic)
	}

	expectedPartition := int32(rawPartition)
	actualPartition := cp.Partition()
	if actualPartition != expectedPartition {
		t.Fatalf("Got the wrong partition, expected %d, got %d instead", expectedPartition, actualPartition)
	}
}
