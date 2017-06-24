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

import "fmt"

const defaultPartition = 0

// channel identifies the Kafka partition the Kafka-based orderer interacts
// with.
type channel interface {
	topic() string
	partition() int32
	fmt.Stringer
}

type channelImpl struct {
	tpc string
	prt int32
}

// Returns a new channel for a given topic name and partition number.
func newChannel(topic string, partition int32) channel {
	return &channelImpl{
		tpc: fmt.Sprintf("%s", topic),
		prt: partition,
	}
}

// topic returns the Kafka topic this channel belongs to.
func (chn *channelImpl) topic() string {
	return chn.tpc
}

// partition returns the Kafka partition where this channel resides.
func (chn *channelImpl) partition() int32 {
	return chn.prt
}

// String returns a string identifying the Kafka topic/partition corresponding
// to this channel.
func (chn *channelImpl) String() string {
	return fmt.Sprintf("%s/%d", chn.tpc, chn.prt)
}
