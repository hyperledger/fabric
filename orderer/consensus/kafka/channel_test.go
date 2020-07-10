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

	"github.com/stretchr/testify/require"
)

func TestChannel(t *testing.T) {
	chn := newChannel(channelNameForTest(t), defaultPartition)

	expectedTopic := channelNameForTest(t)
	actualTopic := chn.topic()
	require.Equal(t, expectedTopic, actualTopic, "Got the wrong topic, expected %s, got %s instead", expectedTopic, actualTopic)

	expectedPartition := int32(defaultPartition)
	actualPartition := chn.partition()
	require.Equal(t, expectedPartition, actualPartition, "Got the wrong partition, expected %d, got %d instead", expectedPartition, actualPartition)
}
