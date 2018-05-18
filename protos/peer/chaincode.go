/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

// Name implements the platforms.NameDescriber interface
func (cs *ChaincodeSpec) Name() string {
	if cs.ChaincodeId == nil {
		return ""
	}

	return cs.ChaincodeId.Name
}

func (cs *ChaincodeSpec) Version() string {
	if cs.ChaincodeId == nil {
		return ""
	}

	return cs.ChaincodeId.Version
}

// Path implements the platforms.PathDescriber interface
func (cs *ChaincodeSpec) Path() string {
	if cs.ChaincodeId == nil {
		return ""
	}

	return cs.ChaincodeId.Path
}

func (cs *ChaincodeSpec) CCType() string {
	return cs.Type.String()
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

func (cds *ChaincodeDeploymentSpec) CCType() string {
	if cds.ChaincodeSpec == nil {
		return ""
	}

	return cds.ChaincodeSpec.CCType()
}

func (cds *ChaincodeDeploymentSpec) Name() string {
	if cds.ChaincodeSpec == nil {
		return ""
	}

	return cds.ChaincodeSpec.Name()
}

func (cds *ChaincodeDeploymentSpec) Version() string {
	if cds.ChaincodeSpec == nil {
		return ""
	}

	return cds.ChaincodeSpec.Version()
}
