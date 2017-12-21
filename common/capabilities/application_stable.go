// +build !experimental

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

// HasCapability returns true if the capability is supported by this binary.
func (ap *ApplicationProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	case ApplicationV1_1:
		return true
	case ApplicationPvtDataExperimental:
		return false
	default:
		return false
	}
}
