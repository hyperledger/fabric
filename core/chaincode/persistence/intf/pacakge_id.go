/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

// PackageID encapsulates chaincode ID
type PackageID string

// String returns a string version of the package ID
func (p PackageID) String() string {
	return string(p)
}
