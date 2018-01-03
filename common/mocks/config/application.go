/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import "github.com/hyperledger/fabric/common/channelconfig"

type MockApplication struct {
	CapabilitiesRv channelconfig.ApplicationCapabilities
}

func (m *MockApplication) Organizations() map[string]channelconfig.ApplicationOrg {
	return nil
}

func (m *MockApplication) Capabilities() channelconfig.ApplicationCapabilities {
	return m.CapabilitiesRv
}

type MockApplicationCapabilities struct {
	SupportedRv                  error
	ForbidDuplicateTXIdInBlockRv bool
	ResourcesTreeRv              bool
	PrivateChannelDataRv         bool
	V1_1ValidationRv             bool
}

func (mac *MockApplicationCapabilities) Supported() error {
	return mac.SupportedRv
}

func (mac *MockApplicationCapabilities) ForbidDuplicateTXIdInBlock() bool {
	return mac.ForbidDuplicateTXIdInBlockRv
}

func (mac *MockApplicationCapabilities) ResourcesTree() bool {
	return mac.ResourcesTreeRv
}

func (mac *MockApplicationCapabilities) PrivateChannelData() bool {
	return mac.PrivateChannelDataRv
}

func (mac *MockApplicationCapabilities) V1_1Validation() bool {
	return mac.V1_1ValidationRv
}
