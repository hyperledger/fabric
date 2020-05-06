/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dataformat

import "fmt"

// The data format stored in ledger databases may be changed in a new release to
// support new features. This file defines the constants to check whether or not
// the data format on the ledger matches the CurrentFormat. If not matched, peer start
// will fail and the ledger must be upgraded to the CurrentFormat. If a Fabric version
// does not introduce a new data format, CurrentFormat will remain the same as the latest
// format prior to the Fabric version.
const (
	// PreviousFormat specifies the data format in previous fabric version
	PreviousFormat = ""

	// CurrentFormat specifies the data format in current fabric version
	CurrentFormat = "2.0"
)

// ErrFormatMismatch is returned if it is detected that the version of the format recorded in
// the internal database is different from what is specified in the `Conf` that is used for opening the db
type ErrFormatMismatch struct {
	DBInfo         string
	ExpectedFormat string
	Format         string
}

func (e *ErrFormatMismatch) Error() string {
	return fmt.Sprintf("unexpected format. db info = [%s], data format = [%s], expected format = [%s]",
		e.DBInfo, e.Format, e.ExpectedFormat,
	)
}

// IsVersionMismatch returns true if err is an ErrFormatMismatch
func IsVersionMismatch(err error) bool {
	_, ok := err.(*ErrFormatMismatch)
	return ok
}
