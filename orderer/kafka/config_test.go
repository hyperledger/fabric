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
	"time"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/localconfig"
)

var (
	brokerID     = int32(0)
	oldestOffset = int64(100)                            // The oldest block available on the broker
	newestOffset = int64(1100)                           // The offset that will be assigned to the next block
	middleOffset = (oldestOffset + newestOffset - 1) / 2 // Just an offset in the middle

	// Amount of time to wait for block processing when doing time-based tests
	// We generally want this value to be as small as possible so as to make tests execute faster
	// But this may have to be bumped up in slower machines
	timePadding = 200 * time.Millisecond
)

var testConf = &config.TopLevel{
	General: config.General{
		OrdererType:   "kafka",
		BatchTimeout:  500 * time.Millisecond,
		BatchSize:     100,
		QueueSize:     100,
		MaxWindowSize: 100,
		ListenAddress: "127.0.0.1",
		ListenPort:    7050,
	},
	Kafka: config.Kafka{
		Brokers:     []string{"127.0.0.1:9092"},
		Topic:       "test",
		PartitionID: 0,
		Version:     sarama.V0_9_0_1,
	},
}
