/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

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
// failed endrosement policy check
type VSCCEndorsementPolicyError struct {
	Reason string
}

// Error returns reasons which lead to the failure
func (e VSCCEndorsementPolicyError) Error() string {
	return e.Reason
}

// VSCCExecutionFailureError error to indicate
// failure during attempt of executing VSCC
// endorsement policy check
type VSCCExecutionFailureError struct {
	Reason string
}

// Error returns reasons which lead to the failure
func (e VSCCExecutionFailureError) Error() string {
	return e.Reason
}
