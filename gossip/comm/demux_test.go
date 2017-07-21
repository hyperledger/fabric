/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "testing"

func TestChannelDeMultiplexer_Close(t *testing.T) {
	demux := NewChannelDemultiplexer()
	demux.Close()
	demux.DeMultiplex("msg")
}
