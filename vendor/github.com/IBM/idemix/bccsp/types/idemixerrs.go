/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package types

import (
	"fmt"
)

type IdemixIssuerPublicKeyImporterErrorType int

const (
	IdemixIssuerPublicKeyImporterUnmarshallingError IdemixIssuerPublicKeyImporterErrorType = iota
	IdemixIssuerPublicKeyImporterHashError
	IdemixIssuerPublicKeyImporterValidationError
	IdemixIssuerPublicKeyImporterNumAttributesError
	IdemixIssuerPublicKeyImporterAttributeNameError
)

type IdemixIssuerPublicKeyImporterError struct {
	Type     IdemixIssuerPublicKeyImporterErrorType
	ErrorMsg string
	Cause    error
}

func (r *IdemixIssuerPublicKeyImporterError) Error() string {
	if r.Cause != nil {
		return fmt.Sprintf("%s: %s", r.ErrorMsg, r.Cause)
	}

	return r.ErrorMsg
}
