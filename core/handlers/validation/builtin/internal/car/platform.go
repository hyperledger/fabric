/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package car provides a representation of the CAR platform for v1.2 and v1.3
// validators.  Support for CAR was removed in v2.0 but validation logic for v1.2
// and v1.3 validates treats CAR as a valid platform.

package car

const errMsg = "CAR packages are no longer supported.  You must upgrade your chaincode and use a supported type."

type Platform struct{}

func (p *Platform) Name() string {
	return "CAR"
}

func (p *Platform) ValidatePath(path string) error {
	return nil
}

func (p *Platform) ValidateCodePackage(code []byte) error {
	return nil
}

func (p *Platform) GetDeploymentPayload(path string) ([]byte, error) {
	return nil, nil
}
