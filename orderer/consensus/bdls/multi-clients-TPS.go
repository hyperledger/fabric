/*
Copyright Ahmed Al Salih. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bdls

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/protoutil"
)

// TestMultiClients function runs multiple clients concurrently
// Submits different envelopes to measure the TPS.
func (c *Chain) TestMultiClients() {
	c.Logger.Info("TEST TPS start")
	time.Sleep(8 * time.Second)
	// start := time.Now()
	// c.Logger.Debugf("TEST TPS start:", start)
	multichannel.SetTPSStart()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go c.TestOrderClient1(wg)
	///go c.TestOrderClient2(wg)
	//go c.TestOrderClient3(wg)
	//go c.TestOrderClient4(wg)
	wg.Wait()

	/*end := time.Now()

	total := end.Sub(start)
	tps := float64(10000) / (float64(total) * math.Pow(10, -9))
	c.TPS = tps
	c.Logger.Infof("**TEST** The total time of execution is: %v with TPS: %f **TEST**", total, tps)*/
}

func (c *Chain) TestOrderClient1(wg *sync.WaitGroup) {
	c.Logger.Infof("For client %v", 1)
	for i := 0; i < 100000; i++ {
		env := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: c.Channel})},
				Data:   []byte(fmt.Sprintf("TEST_MESSAGE-UNCC-Client-1-%v", i)),
			}),
		}
		c.Order(env, 0)
	}
	wg.Done()
}

// This test will run after 20 second for network healthchck after TCP IO error being generated
func (c *Chain) TestOrderClient2(wg *sync.WaitGroup) {
	//time.Sleep(1000 * time.Millisecond)
	c.Logger.Infof("For client %v", 2)
	for i := 0; i < 2500; i++ {
		env := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: c.Channel})},
				Data:   []byte(fmt.Sprintf("TEST_MESSAGE-UNCC-Client-2-%v", i)),
			}),
		}
		c.Order(env, 0)
	}
	wg.Done()
}

// This test will run after 20 second for network healthchck after TCP IO error being generated
func (c *Chain) TestOrderClient3(wg *sync.WaitGroup) {
	//time.Sleep(1000 * time.Millisecond)
	c.Logger.Infof("For client %v", 3)
	for i := 0; i < 2500; i++ {
		env := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: c.Channel})},
				Data:   []byte(fmt.Sprintf("TEST_MESSAGE-UNCC-Client-3-%v", i)),
			}),
		}
		c.Order(env, 0)
	}
	wg.Done()
}

// This test will run after 20 second for network healthchck after TCP IO error being generated
func (c *Chain) TestOrderClient4(wg *sync.WaitGroup) {
	//time.Sleep(1000 * time.Millisecond)
	c.Logger.Infof("For client %v", 3)
	for i := 0; i < 2500; i++ {
		env := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: c.Channel})},
				Data:   []byte(fmt.Sprintf("TEST_MESSAGE-UNCC-Client-4-%v", i)),
			}),
		}
		c.Order(env, 0)
	}
	wg.Done()
}
