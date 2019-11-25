/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"io"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

// MoveOrCopyDir attempts to copy src to dest by firstly trying to move, then copy upon failure.
func MoveOrCopyDir(logger *flogging.FabricLogger, srcroot, destroot string) error {
	mvErr := os.Rename(srcroot, destroot)
	if mvErr == nil {
		return nil
	}

	logger.Debugf("Failed to move %s to %s: %s, try copy instead", srcroot, destroot, mvErr)

	info, err := os.Stat(srcroot)
	if err != nil {
		return errors.WithMessagef(err, "failed to stat dir: %s", srcroot)
	}

	if err = os.MkdirAll(destroot, info.Mode()); err != nil {
		return errors.WithMessagef(err, "failed to make dir: %s", destroot)
	}

	cpErr := CopyDir(srcroot, destroot)
	if cpErr == nil {
		return nil
	}

	logger.Errorf("Failed to copy %s to %s: %s", srcroot, destroot, cpErr)

	rmErr := os.RemoveAll(destroot)
	if rmErr != nil {
		logger.Errorf("Failed to clean targeting dir %s: %s", destroot, rmErr)
	}

	return errors.WithMessagef(cpErr, "failed to copy %s to %s", srcroot, destroot)
}

// CopyDir creates a copy of a dir
func CopyDir(srcroot, destroot string) error {
	return filepath.Walk(srcroot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		srcsubpath, err := filepath.Rel(srcroot, path)
		if err != nil {
			return err
		}
		destpath := filepath.Join(destroot, srcsubpath)

		if info.IsDir() { // its a dir, make corresponding dir in the dest
			if err = os.MkdirAll(destpath, info.Mode()); err != nil {
				return err
			}
			return nil
		}

		// its a file, copy to corresponding path in the dest.
		// Intermediate directories are ensured to exist because parent
		// node is always visited before children in `filepath.Walk`.
		if err = copyFile(path, destpath); err != nil {
			return err
		}
		return nil
	})
}

func copyFile(srcpath, destpath string) error {
	srcFile, err := os.Open(srcpath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.Create(destpath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if err = os.Chmod(destFile.Name(), info.Mode()); err != nil {
		return err
	}

	if _, err = io.Copy(destFile, srcFile); err != nil {
		return err
	}

	return nil
}
