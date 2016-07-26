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

package util

import (
	"fmt"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric/protos"
)

func TestFanIn(t *testing.T) {
	Channels := 500
	Messages := 100

	fh := NewMessageFan()

	for i := 0; i < Channels; i++ {
		c := make(chan *Message, Messages/2)
		pid := &pb.PeerID{Name: fmt.Sprintf("%d", i)}
		fh.RegisterChannel(pid, c)
		go func() {
			for j := 0; j < Messages; j++ {
				c <- &Message{}
			}
		}()
	}

	r := fh.GetOutChannel()

	count := 0
	for {
		select {
		case <-r:
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting to read %d messages from channel, at message %d", Channels*Messages, count)
		}
		count++
		if count == Channels*Messages {
			break
		}
	}

	select {
	case <-r:
		t.Fatalf("Read more than %d messages from channel", Channels*Messages)
	default:
	}

}

func TestFanChannelClose(t *testing.T) {
	fh := NewMessageFan()
	c := make(chan *Message)
	pid := &pb.PeerID{Name: "1"}
	fh.RegisterChannel(pid, c)
	close(c)

	for i := 0; i < 100; i++ {
		if len(fh.ins) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("Channel was not cleaned up")
}
