/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

type MockApplicationCapabilities struct {
	SupportedRv                  error
	ForbidDuplicateTXIdInBlockRv bool
}

func (mac *MockApplicationCapabilities) Supported() error {
	return mac.SupportedRv
}

func (mac *MockApplicationCapabilities) ForbidDuplicateTXIdInBlock() bool {
	return mac.ForbidDuplicateTXIdInBlockRv
}
