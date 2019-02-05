/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/protos/gossip"
)

// InternalEndpoint returns the internal endpoint
// in the secret envelope, or an empty string
// if a failure occurs.
func InternalEndpoint(s *gossip.SecretEnvelope) string {
	secret := &gossip.Secret{}
	if err := proto.Unmarshal(s.Payload, secret); err != nil {
		return ""
	}
	return secret.GetInternalEndpoint()
}
