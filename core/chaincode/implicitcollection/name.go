/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package implicitcollection

import (
	"strings"
)

const (
	prefix = "_implicit_org_"
)

// NameForOrg constructs the name of the implicit collection for the specified org
func NameForOrg(mspid string) string {
	return prefix + mspid
}

// MspIDIfImplicitCollection returns <true, mspid> if the specified name is a valid implicit collection name
// else it returns <false, "">
func MspIDIfImplicitCollection(collectionName string) (isImplicitCollection bool, mspid string) {
	if !strings.HasPrefix(collectionName, prefix) {
		return false, ""
	}
	return true, collectionName[len(prefix):]
}

func IsImplicitCollection(collectionName string) bool {
	return strings.HasPrefix(collectionName, prefix)
}
