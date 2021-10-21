/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

// SyncDir this is a noop on windows
func SyncDir(dirPath string) error {
	return nil
}
