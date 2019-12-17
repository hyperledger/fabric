/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

// LoadPackage loads a chaincode package from the file system
func LoadPackage(ccNameVersion string, path string, getHasher GetHasher) (CCPackage, error) {
	return (&CCInfoFSImpl{GetHasher: getHasher}).GetChaincodeFromPath(ccNameVersion, path)
}
