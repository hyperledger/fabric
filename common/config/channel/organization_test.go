/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"
)

func TestOrganization(t *testing.T) {
	_ = Org(&OrganizationConfig{})
}
