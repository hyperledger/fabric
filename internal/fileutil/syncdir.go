//go:build !windows
// +build !windows

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

import (
	"os"

	"github.com/pkg/errors"
)

// SyncDir fsyncs the given dir
func SyncDir(dirPath string) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return errors.Wrapf(err, "error while opening dir:%s", dirPath)
	}
	if err := dir.Sync(); err != nil {
		dir.Close()
		return errors.Wrapf(err, "error while synching dir:%s", dirPath)
	}
	if err := dir.Close(); err != nil {
		return errors.Wrapf(err, "error while closing dir:%s", dirPath)
	}
	return err
}
