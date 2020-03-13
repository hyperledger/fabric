/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dataformat

import "fmt"

const (
	// Version1x specifies the data format in version 1.x
	Version1x = ""

	// Version20 specifies the data format in version 2.0
	Version20 = "2.0"
)

// ErrVersionMismatch is returned if it is detected that the version of the format recorded in
// the internal database is different from what is specified in the `Conf` that is used for opening the db
type ErrVersionMismatch struct {
	DBInfo          string
	ExpectedVersion string
	Version         string
}

func (e *ErrVersionMismatch) Error() string {
	return fmt.Sprintf("unexpected format. db info = [%s], data format = [%s], expected format = [%s]",
		e.DBInfo, e.Version, e.ExpectedVersion,
	)
}

// IsVersionMismatch returns true if err is an ErrVersionMismatch
func IsVersionMismatch(err error) bool {
	_, ok := err.(*ErrVersionMismatch)
	return ok
}
