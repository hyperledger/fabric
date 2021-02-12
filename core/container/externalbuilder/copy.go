/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

// CopyDir creates a copy of a dir
func CopyDir(logger *flogging.FabricLogger, srcroot, destroot string) error {
	err := filepath.Walk(srcroot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		srcsubpath, err := filepath.Rel(srcroot, path)
		if err != nil {
			return err
		}
		destpath := filepath.Join(destroot, srcsubpath)

		switch {
		case info.IsDir():
			return os.MkdirAll(destpath, info.Mode())
		case info.Mode()&os.ModeSymlink == os.ModeSymlink:
			// filepath.Walk does not follow symbolic links; we need to copy
			// symbolic links as-is because some chaincode types (Node.js) rely
			// on the use of symbolic links.
			return copySymlink(srcroot, path, destpath)
		case info.Mode().IsRegular():
			// Intermediate directories are ensured to exist because parent
			// node is always visited before children in `filepath.Walk`.
			return copyFile(path, destpath)
		default:
			// It's something else that we don't support copying (device, socket, etc)
			return errors.Errorf("refusing to copy unsupported file %s with mode %o", path, info.Mode())
		}
	})
	// If an error occurred, clean up any created files.
	if err != nil {
		if err := os.RemoveAll(destroot); err != nil {
			logger.Errorf("failed to remove destination directory %s after copy error: %s", destroot, err)
		}
		return errors.WithMessagef(err, "failed to copy %s to %s", srcroot, destroot)
	}
	return nil
}

func copySymlink(srcroot, srcpath, destpath string) error {
	// If the symlink is absolute, then we do not want to copy it.
	symlinkDest, err := os.Readlink(srcpath)
	if err != nil {
		return err
	}
	if filepath.IsAbs(symlinkDest) {
		return errors.Errorf("refusing to copy absolute symlink %s -> %s", srcpath, symlinkDest)
	}

	// Determine where the symlink points to. If it points outside
	// of the source root, then we do not want to copy it.
	symlinkDir := filepath.Dir(srcpath)
	symlinkTarget := filepath.Clean(filepath.Join(symlinkDir, symlinkDest))
	relativeTarget, err := filepath.Rel(srcroot, symlinkTarget)
	if err != nil {
		return err
	}
	if relativeTargetElements := strings.Split(relativeTarget, string(os.PathSeparator)); len(relativeTargetElements) >= 1 && relativeTargetElements[0] == ".." {
		return errors.Errorf("refusing to copy symlink %s -> %s pointing outside of source root", srcpath, symlinkDest)
	}

	return os.Symlink(symlinkDest, destpath)
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
