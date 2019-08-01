/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import "github.com/hyperledger/fabric/bccsp/factory"

// LoadPackage loads a chaincode package from the file system
func LoadPackage(ccNameVersion string, path string) (CCPackage, error) {
	return (&CCInfoFSImpl{GetHasher: factory.GetDefault()}).GetChaincodeFromPath(ccNameVersion, path)
}
