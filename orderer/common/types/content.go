/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package types

// JoinBody is encoded in the body of a Join POST request when Content-Type is application/json.
type JoinBody struct {
	// The configuration block (which is a proto.Message), marshaled to bytes.
	// See: `Block` from `github.com/hyperledger/fabric-protos-go/common/`
	ConfigBlock []byte `json:"configBlock"`
}
