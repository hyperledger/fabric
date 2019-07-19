/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"path/filepath"
	"strings"
)

// ValidPath checks to see if the path is absolute, or if it is a
// relative path higher in the tree.  In these cases it returns false.
func ValidPath(uncleanPath string) bool {
	// sanitizedPath will eliminate non-prefix instances of '..', as well
	// as strip './'
	sanitizedPath := filepath.Clean(uncleanPath)

	switch {
	case filepath.IsAbs(sanitizedPath):
		return false
	case strings.HasPrefix(sanitizedPath, ".."):
		return false
	default:
		// Path appears to be relative without escaping higher
		return true
	}
}
