/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import "github.com/hyperledger/fabric/common/channelconfig"

//go:generate mockery -dir . -name Validator -case underscore -output mocks
//go:generate mockery -dir . -name CapabilityProvider -case underscore -output mocks
//go:generate mockery -dir . -name ApplicationCapabilities -case underscore -output mocks

type ApplicationCapabilities interface {
	channelconfig.ApplicationCapabilities
}
