/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func createAndSyncFileAtomically(dir, tmpFile, finalFile string, content []byte) error {
	tempFilePath := filepath.Join(dir, tmpFile)
	finalFilePath := filepath.Join(dir, finalFile)
	if err := createAndSyncFile(tempFilePath, content); err != nil {
		return err
	}
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return err
	}
	return syncDir(dir)
}

func createAndSyncFile(filePath string, content []byte) error {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return errors.Wrapf(err, "error while creating file:%s", filePath)
	}
	_, err = file.Write(content)
	if err != nil {
		file.Close()
		return errors.Wrapf(err, "error while writing to file:%s", filePath)
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return errors.Wrapf(err, "error while synching the file:%s", filePath)
	}
	if err := file.Close(); err != nil {
		return errors.Wrapf(err, "error while closing the file:%s", filePath)
	}
	return nil
}

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
