/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/pkg/errors"

	"github.com/stretchr/testify/require"
)

func TestSyncDir(t *testing.T) {
	t.Run("green-path", func(t *testing.T) {
		testPath := testPath(t)
		defer os.RemoveAll(testPath)

		require.NoError(t, syncDir(testPath))
		require.NoError(t, syncDir(filepath.Dir(testPath)))
	})

	t.Run("non-existent-dir", func(t *testing.T) {
		require.EqualError(t, syncDir("non-existent-dir"), "error while opening dir:non-existent-dir: open non-existent-dir: no such file or directory")
	})
}

func TestRemoveContents(t *testing.T) {
	t.Run("non-empty-dir", func(t *testing.T) {
		testPath := testPath(t)
		defer os.RemoveAll(testPath)

		// create files and a non-empty subdir under testPath to test RemoveContents
		require.NoError(t, createAndSyncFile(filepath.Join(testPath, "file1"), []byte("test-removecontents"), 0644))
		require.NoError(t, createAndSyncFile(filepath.Join(testPath, "file2"), []byte("test-removecontents"), 0644))
		require.NoError(t, os.MkdirAll(filepath.Join(testPath, "non-empty-dir", "some-random-dir"), 0755))
		require.NoError(t, ioutil.WriteFile(filepath.Join(testPath, "non-empty-dir", "some-random-file"), []byte("test-subdir-removecontents"), 0644))

		require.NoError(t, RemoveContents(testPath))
		empty, err := util.DirEmpty(testPath)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("empty-dir", func(t *testing.T) {
		testPath := testPath(t)
		defer os.RemoveAll(testPath)

		require.NoError(t, RemoveContents(testPath))
		empty, err := util.DirEmpty(testPath)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("non-existent-dir", func(t *testing.T) {
		testPath := testPath(t)
		defer os.RemoveAll(testPath)
		require.NoError(t, RemoveContents(filepath.Join(testPath, "non-existent-dir")))
	})
}

func testPath(t *testing.T) string {
	path, err := ioutil.TempDir("", "fileutiltest-")
	require.NoError(t, err)
	return path
}

// CreateAndSyncFile creates a file, writes the content and syncs the file
func createAndSyncFile(filePath string, content []byte, perm os.FileMode) error {
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
