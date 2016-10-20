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

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
)

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestClientDeliverSeekWrong(t *testing.T) {
	t.Run("out-of-range-1", testClientDeliverSeekWrongFunc(uint64(oldestOffset)-1, 10))
	t.Run("out-of-range-2", testClientDeliverSeekWrongFunc(uint64(newestOffset), 10))
	t.Run("bad-window-1", testClientDeliverSeekWrongFunc(uint64(oldestOffset), 0))
	t.Run("bad-window-2", testClientDeliverSeekWrongFunc(uint64(oldestOffset), uint64(testConf.General.MaxWindowSize+1)))
} */

func testClientDeliverSeekWrongFunc(seek, window uint64) func(t *testing.T) {
	return func(t *testing.T) {
		mds := newMockDeliverStream(t)

		dc := make(chan struct{})
		defer close(dc) // Kill the getBlocks goroutine

		mcd := mockNewClientDeliverer(t, testConf, dc)
		defer testClose(t, mcd)
		go func() {
			if err := mcd.Deliver(mds); err == nil {
				t.Fatal("Should have received an error response")
			}
		}()

		mds.incoming <- testNewSeekMessage("specific", seek, window)

		for {
			select {
			case msg := <-mds.outgoing:
				switch msg.GetType().(type) {
				case *ab.DeliverResponse_Error:
					return // This is the success path for this test
				default:
					t.Fatal("Should have received an error response")
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("Should have received an error response")
			}
		}
	}
}

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestClientDeliverSeek(t *testing.T) {
	t.Run("oldest", testClientDeliverSeekFunc("oldest", 0, 10, 10))
	t.Run("in-between", testClientDeliverSeekFunc("specific", uint64(middleOffset), 10, 10))
	t.Run("newest", testClientDeliverSeekFunc("newest", 0, 10, 1))
} */

func testClientDeliverSeekFunc(label string, seek, window uint64, expected int) func(*testing.T) {
	return func(t *testing.T) {
		mds := newMockDeliverStream(t)

		dc := make(chan struct{})
		defer close(dc) // Kill the getBlocks goroutine

		mcd := mockNewClientDeliverer(t, testConf, dc)
		defer testClose(t, mcd)
		go func() {
			if err := mcd.Deliver(mds); err != nil {
				t.Fatal("Deliver error:", err)
			}
		}()

		count := 0
		mds.incoming <- testNewSeekMessage(label, seek, window)
		for {
			select {
			case <-mds.outgoing:
				count++
				if count > expected {
					t.Fatalf("Delivered %d blocks to the client w/o ACK, expected %d", count, expected)
				}
			case <-time.After(500 * time.Millisecond):
				if count != expected {
					t.Fatalf("Delivered %d blocks to the client w/o ACK, expected %d", count, expected)
				}
				return
			}
		}
	}
}

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestClientDeliverAckWrong(t *testing.T) {
	t.Run("out-of-range-ack-1", testClientDeliverAckWrongFunc(uint64(middleOffset)-2))
	t.Run("out-of-range-ack-2", testClientDeliverAckWrongFunc(uint64(newestOffset)))
} */

func testClientDeliverAckWrongFunc(ack uint64) func(t *testing.T) {
	return func(t *testing.T) {
		mds := newMockDeliverStream(t)

		dc := make(chan struct{})
		defer close(dc) // Kill the getBlocks goroutine

		mcd := mockNewClientDeliverer(t, testConf, dc)
		defer testClose(t, mcd)
		go func() {
			if err := mcd.Deliver(mds); err == nil {
				t.Fatal("Should have received an error response")
			}
		}()

		mds.incoming <- testNewSeekMessage("specific", uint64(middleOffset), 10)
		mds.incoming <- testNewAckMessage(ack)
		for {
			select {
			case msg := <-mds.outgoing:
				switch msg.GetType().(type) {
				case *ab.DeliverResponse_Error:
					return // This is the success path for this test
				default:
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("Should have returned earlier due to wrong ACK")
			}
		}
	}
}

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestClientDeliverAck(t *testing.T) {
	t.Run("in-between", testClientDeliverAckFunc("specific", uint64(middleOffset), 10, 10, 2*10))
	t.Run("newest", testClientDeliverAckFunc("newest", 0, 10, 1, 1))
} */

func testClientDeliverAckFunc(label string, seek, window uint64, threshold, expected int) func(t *testing.T) {
	return func(t *testing.T) {
		mds := newMockDeliverStream(t)

		dc := make(chan struct{})
		defer close(dc) // Kill the getBlocks goroutine

		mcd := mockNewClientDeliverer(t, testConf, dc)
		defer testClose(t, mcd)
		go func() {
			if err := mcd.Deliver(mds); err != nil {
				t.Fatal("Deliver error:", err)
			}
		}()

		mds.incoming <- testNewSeekMessage(label, seek, window)
		count := 0
		for {
			select {
			case msg := <-mds.outgoing:
				count++
				if count == threshold {
					mds.incoming <- testNewAckMessage(msg.GetBlock().Number)
				}
				if count > expected {
					t.Fatalf("Delivered %d blocks to the client w/o ACK, expected %d", count, expected)
				}
			case <-time.After(500 * time.Millisecond):
				if count != expected {
					t.Fatalf("Delivered %d blocks to the client w/o ACK, expected %d", count, expected)
				}
				return
			}
		}
	}
}
