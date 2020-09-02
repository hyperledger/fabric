/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// RemoveContents removes all the files and subdirs under the specified directory.
// It returns nil if the specified directory does not exist.
func RemoveContents(dir string) error {
	contents, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "error reading directory %s", dir)
	}

	for _, c := range contents {
		if err = os.RemoveAll(filepath.Join(dir, c.Name())); err != nil {
			return errors.Wrapf(err, "error removing %s under directory %s", c.Name(), dir)
		}
	}
	return syncDir(dir)
}

// SyncDir fsyncs the given dir
func syncDir(dirPath string) error {
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
