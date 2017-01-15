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

	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestProducerSend(t *testing.T) {
	mp := mockNewProducer(t, cp, testMiddleOffset, make(chan *ab.KafkaMessage))
	defer testClose(t, mp)

	go func() {
		<-mp.(*mockProducerImpl).disk // Retrieve the message that we'll be sending below
	}()

	if err := mp.Send(cp, utils.MarshalOrPanic(newRegularMessage([]byte("foo")))); err != nil {
		t.Fatalf("Mock producer was not initialized correctly: %s", err)
	}
}
