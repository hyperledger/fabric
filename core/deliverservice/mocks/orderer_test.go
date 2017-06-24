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
	"math"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type clStream struct {
	grpc.ServerStream
}

func (cs *clStream) Send(*orderer.DeliverResponse) error {
	return nil
}
func (cs *clStream) Recv() (*common.Envelope, error) {
	seekInfo := &orderer.SeekInfo{
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}
	si, _ := pb.Marshal(seekInfo)
	payload := &common.Payload{}
	payload.Data = si
	b, err := pb.Marshal(payload)
	if err != nil {
		panic(err)
	}
	e := &common.Envelope{Payload: b}
	return e, nil
}

func TestOrderer(t *testing.T) {
	o := NewOrderer(8000, t)

	go func() {
		time.Sleep(time.Second)
		o.SendBlock(uint64(0))
		o.Shutdown()
	}()

	assert.Panics(t, func() {
		o.Broadcast(nil)
	})
	o.SetNextExpectedSeek(uint64(0))
	o.Deliver(&clStream{})
}
