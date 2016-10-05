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
)

func TestDeliverMultipleClients(t *testing.T) {
	connectedClients := 3
	seekMsgs := []struct {
		start        string
		seek, window uint64
	}{
		{"oldest", 0, 10}, {"newest", 0, 10}, {"specific", uint64(middleOffset), 10},
	}
	expected := 21 // 10 + 1 + 10

	md := mockNewDeliverer(t, testConf)
	defer testClose(t, md)

	var mds []*mockDeliverStream
	for i := 0; i < connectedClients; i++ {
		mds = append(mds, newMockDeliverStream(t))
		go func() {
			if err := md.Deliver(mds[i]); err != nil {
				t.Fatal("Deliver error:", err)
			}
		}()
		mds[i].incoming <- testNewSeekMessage(seekMsgs[i].start, seekMsgs[i].seek, seekMsgs[i].window)
	}

	count := 0

	for i := 0; i < connectedClients; i++ {
	client:
		for {
			select {
			case <-mds[i].outgoing:
				count++
			case <-time.After(500 * time.Millisecond):
				break client
			}
		}
	}

	if count != expected {
		t.Fatalf("Expected %d blocks total delivered to all clients, got %d", expected, count)
	}
}

func TestDeliverClose(t *testing.T) {
	errChan := make(chan error)

	md := mockNewDeliverer(t, testConf)
	mds := newMockDeliverStream(t)
	go func() {
		if err := md.Deliver(mds); err != nil {
			t.Fatal("Deliver error:", err)
		}
	}()

	go func() {
		errChan <- md.Close()
	}()

	for {
		select {
		case err := <-errChan:
			if err != nil {
				t.Fatal("Error when closing the deliverer:", err)
			}
			return
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Deliverer should have closed all client deliverers by now")
		}
	}

}
