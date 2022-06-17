/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileExists(t *testing.T) {
	t.Run("non-existent-file", func(t *testing.T) {
		exists, size, err := FileExists("/non-existent-file")
		require.NoError(t, err)
		require.False(t, exists)
		require.Equal(t, int64(0), size)
	})

	t.Run("dir-path", func(t *testing.T) {
		testPath := t.TempDir()

		exists, size, err := FileExists(testPath)
		require.EqualError(t, err, fmt.Sprintf("the supplied path [%s] is a dir", testPath))
		require.False(t, exists)
		require.Equal(t, int64(0), size)
	})

	t.Run("empty-file", func(t *testing.T) {
		testPath := t.TempDir()

		file := filepath.Join(testPath, "empty-file")
		f, err := os.Create(file)
		require.NoError(t, err)
		defer f.Close()

		exists, size, err := FileExists(file)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, int64(0), size)
	})

	t.Run("file-with-content", func(t *testing.T) {
		testPath := t.TempDir()

		file := filepath.Join(testPath, "empty-file")
		contents := []byte("some random contents")
		ioutil.WriteFile(file, []byte(contents), 0o644)
		exists, size, err := FileExists(file)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, int64(len(contents)), size)
	})
}

func TestDirExists(t *testing.T) {
	t.Run("non-existent-path", func(t *testing.T) {
		testPath := t.TempDir()

		exists, err := DirExists(filepath.Join(testPath, "non-existent-path"))
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("dir-exists", func(t *testing.T) {
		testPath := t.TempDir()

		exists, err := DirExists(testPath)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("file-exists", func(t *testing.T) {
		testPath := t.TempDir()

		file := filepath.Join(testPath, "empty-file")
		f, err := os.Create(file)
		require.NoError(t, err)
		defer f.Close()

		exists, err := DirExists(file)
		require.EqualError(t, err, fmt.Sprintf("the supplied path [%s] exists but is not a dir", file))
		require.False(t, exists)
	})
}

func TestDirEmpty(t *testing.T) {
	t.Run("non-existent-dir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "non-existent-dir")
		_, err := DirEmpty(dir)
		require.EqualError(t, err, fmt.Sprintf("error opening dir [%s]: open %s: no such file or directory", dir, dir))
	})

	t.Run("empty-dir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "empty-dir")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		empty, err := DirEmpty(dir)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("dir-has-file", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "non-empty-dir")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		file := filepath.Join(testPath, "non-empty-dir", "some-random-file")
		require.NoError(t, ioutil.WriteFile(file, []byte("some-random-text"), 0o644))
		empty, err := DirEmpty(dir)
		require.NoError(t, err)
		require.False(t, empty)
	})

	t.Run("dir-has-subdir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "non-empty-dir")
		subdir := filepath.Join(testPath, "non-empty-dir", "some-random-dir")
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		empty, err := DirEmpty(dir)
		require.NoError(t, err)
		require.False(t, empty)
	})
}

func TestCreateDirIfMissing(t *testing.T) {
	t.Run("non-existent-dir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "non-existent-dir")
		empty, err := CreateDirIfMissing(dir)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("existing-dir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "empty-dir")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		empty, err := CreateDirIfMissing(dir)
		require.NoError(t, err)
		require.True(t, empty)

		require.NoError(t, ioutil.WriteFile(filepath.Join(dir, "some-random-file"), []byte("some-random-text"), 0o644))
		empty, err = CreateDirIfMissing(dir)
		require.NoError(t, err)
		require.False(t, empty)
	})

	t.Run("cannot-create-dir", func(t *testing.T) {
		testPath := t.TempDir()

		path := filepath.Join(testPath, "some-random-file")
		require.NoError(t, ioutil.WriteFile(path, []byte("some-random-text"), 0o644))
		empty, err := CreateDirIfMissing(path)
		require.EqualError(t, err, fmt.Sprintf("error while creating dir: %s: mkdir %s: not a directory", path, path))
		require.False(t, empty)
	})
}

func TestListSubdirs(t *testing.T) {
	t.Run("only-subdirs", func(t *testing.T) {
		testPath := t.TempDir()

		childFolders := []string{".childFolder1", "childFolder2", "childFolder3"}
		for _, folder := range childFolders {
			require.NoError(t, os.MkdirAll(filepath.Join(testPath, folder), 0o755))
		}
		subFolders, err := ListSubdirs(testPath)
		require.NoError(t, err)
		require.Equal(t, subFolders, childFolders)
	})

	t.Run("only-file", func(t *testing.T) {
		testPath := t.TempDir()

		require.NoError(t, ioutil.WriteFile(filepath.Join(testPath, "some-random-file"), []byte("random-text"), 0o644))
		subFolders, err := ListSubdirs(testPath)
		require.NoError(t, err)
		require.Len(t, subFolders, 0)
	})

	t.Run("empty-dir", func(t *testing.T) {
		testPath := t.TempDir()

		subFolders, err := ListSubdirs(testPath)
		require.NoError(t, err)
		require.Len(t, subFolders, 0)
	})

	t.Run("non-existent-dir", func(t *testing.T) {
		testPath := t.TempDir()

		dir := filepath.Join(testPath, "non-existent-dir")
		_, err := ListSubdirs(dir)
		require.EqualError(t, err, fmt.Sprintf("error reading dir %s: open %s: no such file or directory", dir, dir))
	})
}

func TestCreateAndSyncFileAtomically(t *testing.T) {
	t.Run("green-path", func(t *testing.T) {
		testPath := t.TempDir()

		content := []byte("some random content")
		err := CreateAndSyncFileAtomically(testPath, "tmpFile", "finalFile", content, 0o644)
		require.NoError(t, err)
		require.NoFileExists(t, filepath.Join(testPath, "tmpFile"))
		contentRetrieved, err := ioutil.ReadFile(filepath.Join(testPath, "finalFile"))
		require.NoError(t, err)
		require.Equal(t, content, contentRetrieved)
	})

	t.Run("dir-doesnot-exist", func(t *testing.T) {
		testPath := t.TempDir()

		content := []byte("some random content")
		dir := filepath.Join(testPath, "non-exitent-dir")
		tmpFile := filepath.Join(dir, "tmpFile")
		err := CreateAndSyncFileAtomically(dir, "tmpFile", "finalFile", content, 0o644)
		require.EqualError(t, err, fmt.Sprintf("error while creating file:%s: open %s: no such file or directory", tmpFile, tmpFile))
	})

	t.Run("tmp-file-already-exists", func(t *testing.T) {
		testPath := t.TempDir()

		content := []byte("some random content")
		tmpFile := filepath.Join(testPath, "tmpFile")
		err := ioutil.WriteFile(tmpFile, []byte("existing-contents"), 0o644)
		require.NoError(t, err)
		err = CreateAndSyncFileAtomically(testPath, "tmpFile", "finalFile", content, 0o644)
		require.EqualError(t, err, fmt.Sprintf("error while creating file:%s: open %s: file exists", tmpFile, tmpFile))
	})

	t.Run("final-file-already-exists", func(t *testing.T) {
		testPath := t.TempDir()

		content := []byte("some random content")
		finalFile := filepath.Join(testPath, "finalFile")
		err := ioutil.WriteFile(finalFile, []byte("existing-contents"), 0o644)
		require.NoError(t, err)
		err = CreateAndSyncFileAtomically(testPath, "tmpFile", "finalFile", content, 0o644)
		require.NoError(t, err)
		contentRetrieved, err := ioutil.ReadFile(filepath.Join(testPath, "finalFile"))
		require.NoError(t, err)
		require.Equal(t, content, contentRetrieved)
	})

	t.Run("rename-returns-error", func(t *testing.T) {
		testPath := t.TempDir()

		content := []byte("some random content")
		tmpFile := filepath.Join(testPath, "tmpFile")
		finalFile := filepath.Join(testPath, "finalFile")
		require.NoError(t, os.Mkdir(finalFile, 0o644))
		err := CreateAndSyncFileAtomically(testPath, "tmpFile", "finalFile", content, 0o644)
		require.EqualError(t, err, fmt.Sprintf("rename %s %s: file exists", tmpFile, finalFile))
	})
}

func TestSyncDir(t *testing.T) {
	t.Run("green-path", func(t *testing.T) {
		testPath := t.TempDir()

		require.NoError(t, SyncDir(testPath))
		require.NoError(t, SyncParentDir(testPath))
	})

	t.Run("non-existent-dir", func(t *testing.T) {
		require.EqualError(t, SyncDir("non-existent-dir"), "error while opening dir:non-existent-dir: open non-existent-dir: no such file or directory")
	})
}

func TestRemoveContents(t *testing.T) {
	t.Run("non-empty-dir", func(t *testing.T) {
		testPath := t.TempDir()

		// create files and a non-empty subdir under testPath to test RemoveContents
		require.NoError(t, CreateAndSyncFile(filepath.Join(testPath, "file1"), []byte("test-removecontents"), 0o644))
		require.NoError(t, CreateAndSyncFile(filepath.Join(testPath, "file2"), []byte("test-removecontents"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(testPath, "non-empty-dir", "some-random-dir"), 0o755))
		require.NoError(t, ioutil.WriteFile(filepath.Join(testPath, "non-empty-dir", "some-random-file"), []byte("test-subdir-removecontents"), 0o644))

		require.NoError(t, RemoveContents(testPath))
		empty, err := DirEmpty(testPath)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("empty-dir", func(t *testing.T) {
		testPath := t.TempDir()

		require.NoError(t, RemoveContents(testPath))
		empty, err := DirEmpty(testPath)
		require.NoError(t, err)
		require.True(t, empty)
	})

	t.Run("non-existent-dir", func(t *testing.T) {
		testPath := t.TempDir()
		require.NoError(t, RemoveContents(filepath.Join(testPath, "non-existent-dir")))
	})
}
