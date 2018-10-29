/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity_test

import "github.com/hyperledger/fabric/msp"

//go:generate counterfeiter -o mock/issuing_validator.go -fake-name IssuingValidator . IssuingValidator
//go:generate counterfeiter -o mock/public_info.go -fake-name PublicInfo . PublicInfo
//go:generate counterfeiter -o mock/deserializer_manager.go -fake-name DeserializerManager . DeserializerManager
//go:generate counterfeiter -o mock/deserializer.go -fake-name Deserializer . Deserializer
//go:generate counterfeiter -o mock/identity.go -fake-name Identity ./../../msp/ Identity

type Identity interface {
	msp.Identity
}
