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
	"testing"
	"time"

	"github.com/Shopify/sarama"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
)

var (
	testBrokerID     = int32(0)
	testOldestOffset = int64(100)                                    // The oldest block available on the broker
	testNewestOffset = int64(1100)                                   // The offset that will be assigned to the next block
	testMiddleOffset = (testOldestOffset + testNewestOffset - 1) / 2 // Just an offset in the middle

	// Amount of time to wait for block processing when doing time-based tests
	// We generally want this value to be as small as possible so as to make tests execute faster
	// But this may have to be bumped up in slower machines
	testTimePadding = 200 * time.Millisecond
)

var testGenesisConf = &genesisconfig.TopLevel{
	Orderer: &genesisconfig.Orderer{
		Kafka: genesisconfig.Kafka{
			Brokers: []string{"127.0.0.1:9092"},
		},
	},
}

var testConf = &config.TopLevel{
	Kafka: config.Kafka{
		Retry: config.Retry{
			Period: 3 * time.Second,
			Stop:   60 * time.Second,
		},
		Verbose: false,
		Version: sarama.V0_9_0_1,
	},
}

func testClose(t *testing.T, x Closeable) {
	if err := x.Close(); err != nil {
		t.Fatal("Cannot close mock resource:", err)
	}
}

func newTestEnvelope(content string) *cb.Envelope {
	return &cb.Envelope{Payload: []byte(content)}
}
