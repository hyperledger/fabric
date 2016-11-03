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
)

func testClose(t *testing.T, x Closeable) {
	if err := x.Close(); err != nil {
		t.Fatal("Cannot close mock resource:", err)
	}
}

func testNewSeekMessage(startLabel string, seekNo, windowNo uint64) *ab.DeliverUpdate {
	var startVal ab.SeekInfo_StartType
	switch startLabel {
	case "oldest":
		startVal = ab.SeekInfo_OLDEST
	case "newest":
		startVal = ab.SeekInfo_NEWEST
	default:
		startVal = ab.SeekInfo_SPECIFIED

	}
	return &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:           startVal,
				SpecifiedNumber: seekNo,
				WindowSize:      windowNo,
			},
		},
	}
}

func testNewAckMessage(ackNo uint64) *ab.DeliverUpdate {
	return &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Acknowledgement{
			Acknowledgement: &ab.Acknowledgement{
				Number: ackNo,
			},
		},
	}
}
