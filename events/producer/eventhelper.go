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

package producer

import (
	ehpb "github.com/hyperledger/fabric/protos"
)

//CreateBlockEvent creates a Event from a Block
func CreateBlockEvent(te *ehpb.Block) *ehpb.Event {
	return &ehpb.Event{Event: &ehpb.Event_Block{Block: te}}
}

//CreateChaincodeEvent creates a Event from a ChaincodeEvent
func CreateChaincodeEvent(te *ehpb.ChaincodeEvent) *ehpb.Event {
	return &ehpb.Event{Event: &ehpb.Event_ChaincodeEvent{ChaincodeEvent: te}}
}

//CreateRejectionEvent creates an Event from TxResults
func CreateRejectionEvent(tx *ehpb.Transaction, errorMsg string) *ehpb.Event {
	return &ehpb.Event{Event: &ehpb.Event_Rejection{Rejection: &ehpb.Rejection{Tx: tx, ErrorMsg: errorMsg}}}
}
