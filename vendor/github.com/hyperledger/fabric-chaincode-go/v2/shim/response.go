// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

const (
	// OK constant - status code less than 400, endorser will endorse it.
	// OK means init or invoke successfully.
	OK = 200

	// ERRORTHRESHOLD constant - status code greater than or equal to 400 will be considered an error and rejected by endorser.
	ERRORTHRESHOLD = 400

	// ERROR constant - default error value
	ERROR = 500
)

// Success ...
func Success(payload []byte) *peer.Response {
	return &peer.Response{
		Status:  OK,
		Payload: payload,
	}
}

// Error ...
func Error(msg string) *peer.Response {
	return &peer.Response{
		Status:  ERROR,
		Message: msg,
	}
}
