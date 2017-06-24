/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.
 Copyright IBM Corp. 2017 All Rights Reserved.

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

package errors

// A set of constants for error reason codes, which is based on HTTP codes
// http://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml
const (
	// Invalid inputs on API calls
	BadRequest = "400"

	// Forbidden due to access control issues
	Forbidden = "403"

	// Not Found (eg chaincode not found)
	NotFound = "404"

	// Request timeout (chaincode or ledger)
	Timeout = "408"

	// Example, duplicate transactions or replay attacks
	Conflict = "409"

	// Request for resource is not available. Example, a chaincode has
	// been upgraded and the request uses an old version
	Gone = "410"

	// Payload of the request exceeds allowed size
	PayloadTooLarge = "413"

	// Example, marshal/unmarshalling protobuf error
	UnprocessableEntity = "422"

	// Protocol version is no longer supported
	UpgradeRequired = "426"

	// Internal server errors that are not classified below
	Internal = "500"

	// Requested chaincode function has not been implemented
	NotImplemented = "501"

	// Requested chaincode is not available
	Unavailable = "503"

	// File IO errors
	FileIO = "520"

	// Network IO errors
	NetworkIO = "521"
)

// A set of constants for component codes
const (
	// BCCSP is fabic/BCCSP
	BCCSP = "CSP"

	// Common is fabric/common
	Common = "CMN"

	// Core is fabric/core
	Core = "COR"

	// Event is fabric/events component
	Event = "EVT"

	// Gossip is fabric/gossip
	Gossip = "GSP"

	// Ledger is fabric/core/ledger
	Ledger = "LGR"

	// Peer is fabric/peer
	Peer = "PER"

	// Orderer is fabric/orderer
	Orderer = "ORD"

	// MSP is fabric/msp
	MSP = "MSP"

	// ChaincodeSupport is fabric/core/chaincode
	ChaincodeSupport = "CCS"

	// DeliveryService is fabric/core/deliverservice
	DeliveryService = "CDS"

	// SystemChaincode is fabric/core/scc (system chaincode)
	SystemChaincode = "SCC"
)
