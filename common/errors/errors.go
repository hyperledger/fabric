/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

// TxValidationError marks that the error is related to
// validation of a transaction
type TxValidationError interface {
	error
	IsValid() bool
}

// VSCCInfoLookupFailureError error to indicate inability
// to obtain VSCC information from LCCC
type VSCCInfoLookupFailureError struct {
	Reason string
}

// Error returns reasons which lead to the failure
func (e VSCCInfoLookupFailureError) Error() string {
	return e.Reason
}

// VSCCEndorsementPolicyError error to mark transaction
// failed endorsement policy check
type VSCCEndorsementPolicyError struct {
	Err error
}

func (e *VSCCEndorsementPolicyError) IsValid() bool {
	return e.Err == nil
}

// Error returns reasons which lead to the failure
func (e VSCCEndorsementPolicyError) Error() string {
	return e.Err.Error()
}

// VSCCExecutionFailureError error to indicate
// failure during attempt of executing VSCC
// endorsement policy check
type VSCCExecutionFailureError struct {
	Err error
}

// Error returns reasons which lead to the failure
func (e VSCCExecutionFailureError) Error() string {
	return e.Err.Error()
}

func (e *VSCCExecutionFailureError) IsValid() bool {
	return e.Err == nil
}
