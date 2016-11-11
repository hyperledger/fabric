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

//All changes for handling internal events should be in this file
//Step 1 - add the event type name to the const
//Step 2 - add a case statement to getMessageType
//Step 3 - add an AddEventType call to addInternalEventTypes

package producer

import (
	pb "github.com/hyperledger/fabric/protos/peer"
)

//----Event Types -----
/*const (
	RegisterType  = "register"
	RejectionType = "rejection"
	BlockType     = "block"
)*/

func getMessageType(e *pb.Event) pb.EventType {
	switch e.Event.(type) {
	case *pb.Event_Register:
		return pb.EventType_REGISTER
	case *pb.Event_Block:
		return pb.EventType_BLOCK
	case *pb.Event_ChaincodeEvent:
		return pb.EventType_CHAINCODE
	case *pb.Event_Rejection:
		return pb.EventType_REJECTION
	default:
		return -1
	}
}

//should be called at init time to register supported internal events
func addInternalEventTypes() {
	AddEventType(pb.EventType_BLOCK)
	AddEventType(pb.EventType_CHAINCODE)
	AddEventType(pb.EventType_REJECTION)
	AddEventType(pb.EventType_REGISTER)
}
