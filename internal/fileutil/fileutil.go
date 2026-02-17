/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// CreateAndSyncFileAtomically writes the content to the tmpFile, fsyncs the tmpFile,
// renames the tmpFile to the finalFile. In other words, in the event of a crash, either
// the final file will not be visible or it will have the full contents.
// The tmpFile should not be existing. The finalFile, if exists, will be overwritten (default rename behavior)
func CreateAndSyncFileAtomically(dir, tmpFile, finalFile string, content []byte, perm os.FileMode) error {
	tempFilePath := filepath.Join(dir, tmpFile)
	finalFilePath := filepath.Join(dir, finalFile)
	if err := CreateAndSyncFile(tempFilePath, content, perm); err != nil {
		return err
	}
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return err
	}
	return nil
}

// CreateAndSyncFile creates a file, writes the content and syncs the file
func CreateAndSyncFile(filePath string, content []byte, perm os.FileMode) error {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
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

// SyncParentDir fsyncs the parent dir of the given path
func SyncParentDir(path string) error {
	return SyncDir(filepath.Dir(path))
}

// CreateDirIfMissing makes sure that the dir exists and returns whether the dir is empty
func CreateDirIfMissing(dirPath string) (bool, error) {
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return false, errors.Wrapf(err, "error while creating dir: %s", dirPath)
	}
	if err := SyncParentDir(dirPath); err != nil {
		return false, err
	}
	return DirEmpty(dirPath)
}

// DirEmpty returns true if the dir at dirPath is empty
func DirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return false, errors.Wrapf(err, "error opening dir [%s]", dirPath)
	}
	defer f.Close()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	err = errors.Wrapf(err, "error checking if dir [%s] is empty", dirPath)
	return false, err
}

// FileExists checks whether the given file exists.
// If the file exists, this method also returns the size of the file.
func FileExists(path string) (bool, int64, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, errors.Wrapf(err, "error checking if file [%s] exists", path)
	}
	if info.IsDir() {
		return false, 0, errors.Errorf("the supplied path [%s] is a dir", path)
	}
	return true, info.Size(), nil
}

// DirExists returns true if the dir already exists
func DirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "error checking if file [%s] exists", path)
	}
	if !info.IsDir() {
		return false, errors.Errorf("the supplied path [%s] exists but is not a dir", path)
	}
	return true, nil
}

// ListSubdirs returns the subdirectories
func ListSubdirs(dirPath string) ([]string, error) {
	subdirs := []string{}
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading dir %s", dirPath)
	}
	for _, f := range files {
		if f.IsDir() {
			subdirs = append(subdirs, f.Name())
		}
	}
	return subdirs, nil
}

// RemoveContents removes all the files and subdirs under the specified directory.
// It returns nil if the specified directory does not exist.
func RemoveContents(dir string) error {
	contents, err := os.ReadDir(dir)
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
	return SyncDir(dir)
}
