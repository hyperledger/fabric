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
	"testing"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

func mockNewBroadcaster(t *testing.T, conf *config.TopLevel, seek int64, disk chan []byte) Broadcaster {
	genesisBlock, _ := static.New().GenesisBlock()
	wait := make(chan struct{})

	mb := &broadcasterImpl{
		producer:   mockNewProducer(t, conf, seek, disk),
		config:     conf,
		batchChan:  make(chan *cb.Envelope, conf.General.BatchSize),
		messages:   genesisBlock.GetData().Data,
		nextNumber: uint64(seek),
	}

	go func() {
		rxBlockBytes := <-disk
		rxBlock := &cb.Block{}
		if err := proto.Unmarshal(rxBlockBytes, rxBlock); err != nil {
			panic(err)
		}
		if !proto.Equal(rxBlock.GetData(), genesisBlock.GetData()) {
			panic(fmt.Errorf("Broadcaster not functioning as expected"))
		}
		close(wait)
	}()

	mb.once.Do(func() {
		// Send the genesis block to create the topic
		// otherwise consumers will throw an exception.
		mb.sendBlock()
		// Spawn the goroutine that cuts blocks
		go mb.cutBlock(mb.config.General.BatchTimeout, mb.config.General.BatchSize)
	})
	<-wait

	return mb
}
