/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ramledger

import (
	"testing"
)

func TestGetOrCreate(t *testing.T) {
	rlf := New(3)
	channel, err := rlf.GetOrCreate("testchannelid")
	if err != nil {
		panic(err)
	}
	channel2, err := rlf.GetOrCreate("testchannelid")
	if err != nil {
		panic(err)
	}
	if channel != channel2 {
		t.Fatalf("Expecting already created channel.")
	}
}

func TestChannelIDs(t *testing.T) {
	rlf := New(3)
	rlf.GetOrCreate("channel1")
	rlf.GetOrCreate("channel2")
	rlf.GetOrCreate("channel3")
	if len(rlf.ChannelIDs()) != 3 {
		t.Fatalf("Expecting three channels,")
	}
	rlf.Close()
}
