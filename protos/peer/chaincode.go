/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

// Path implements the platforms.PathDescriber interface
func (cs *ChaincodeSpec) Path() string {
	if cs.ChaincodeId == nil {
		return ""
	}

	return cs.ChaincodeId.Path
}

// Path implements the platforms.PathDescriber interface
func (cds *ChaincodeDeploymentSpec) Path() string {
	if cds.ChaincodeSpec == nil {
		return ""
	}

	return cds.ChaincodeSpec.Path()
}

// Bytes implements the platforms.CodePackage interface
func (cds *ChaincodeDeploymentSpec) Bytes() []byte {
	return cds.CodePackage
}
