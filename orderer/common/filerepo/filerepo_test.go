/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filerepo_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/filerepo"
	"github.com/stretchr/testify/require"
)

func TestNewFileRepo(t *testing.T) {
	// Write a temporary file
	tempFilePath := filepath.Join("testdata", "join", "mychannel.join~")
	tempFile, err := os.Create(tempFilePath)
	require.NoError(t, err)
	defer func() {
		tempFile.Close()
		os.Remove(tempFilePath)
	}()

	// Check tempfile exists prior to creating a new file repo
	_, err = os.Stat(tempFilePath)
	require.NoError(t, err)

	_, err = filerepo.New("testdata", "join")
	require.NoError(t, err)

	// Check tempfile was cleared out upon creating a new file repo
	require.NoFileExists(t, tempFilePath)
}

func TestNewFileRepoFailure(t *testing.T) {
	tests := []struct {
		testName    string
		fileSuffix  string
		expectedErr string
	}{
		{
			testName:    "invalid fileSuffix",
			fileSuffix:  "join/block",
			expectedErr: "fileSuffix [join/block] illegal, cannot contain os path separator",
		},
		{
			testName:    "empty fileSuffix",
			fileSuffix:  "",
			expectedErr: "fileSuffix illegal, cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			_, err := filerepo.New("testdata", tt.fileSuffix)
			require.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestFileRepo_Save(t *testing.T) {
	tests := []struct {
		testName string
		content  []byte
	}{
		{
			testName: "Non-empty bytes",
			content:  []byte("block-bytes"),
		},
		{
			testName: "Empty bytes",
			content:  []byte{},
		},
		{
			testName: "Nil bytes",
			content:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			r, err := filerepo.New("testdata", "join")
			require.NoError(t, err)

			filePath := filepath.Join("testdata", "join", "newchannel.join")

			err = r.Save("newchannel", tt.content)
			defer os.Remove(filePath)
			require.NoError(t, err)

			// Check file bytes
			bytes, err := ioutil.ReadFile(filePath)
			require.NoError(t, err)
			if tt.content != nil {
				require.Equal(t, tt.content, bytes)
			} else {
				require.Equal(t, []byte{}, bytes)
			}

			// Check tempfile doesn't exist
			require.NoFileExists(t, filePath+"~")
		})
	}
}

func TestFileRepo_SaveFailure(t *testing.T) {
	r, err := filerepo.New("testdata", "join")
	require.NoError(t, err)

	err = r.Save("mychannel", []byte{})
	require.EqualError(t, err, os.ErrExist.Error())
}

func TestFileRepo_Remove(t *testing.T) {
	// Write a temporary file
	tempFilePath := filepath.Join("testdata", "join", "channel2.join")
	tempFile, err := os.Create(tempFilePath)
	require.NoError(t, err)
	defer func() {
		tempFile.Close()
		os.Remove(tempFilePath)
	}()

	r, err := filerepo.New("testdata", "join")
	require.NoError(t, err)

	err = r.Remove("channel2")
	require.NoError(t, err)

	require.NoFileExists(t, tempFilePath)
}

func TestFileRepo_Read(t *testing.T) {
	t.Run("Successful read, non-empty bytes", func(t *testing.T) {
		r, err := filerepo.New("testdata", "join")
		require.NoError(t, err)

		bytes, err := r.Read("mychannel")
		require.NoError(t, err)
		require.Equal(t, "dummy-data", string(bytes))
	})

	t.Run("Successful read, empty bytes", func(t *testing.T) {
		r, err := filerepo.New("testdata", "remove")
		require.NoError(t, err)

		bytes, err := r.Read("mychannel")
		require.NoError(t, err)
		require.Equal(t, []byte{}, bytes)
	})

	t.Run("Failed read, invalid file", func(t *testing.T) {
		r, err := filerepo.New("testdata", "join")
		require.NoError(t, err)

		_, err = r.Read("invalidfile")
		require.EqualError(t, err, "open testdata/join/invalidfile.join: no such file or directory")
	})
}

func TestFileRepo_List(t *testing.T) {
	r, err := filerepo.New("testdata", "join")
	require.NoError(t, err)

	files, err := r.List()
	require.NoError(t, err)
	require.Equal(t, []string{"mychannel.join"}, files)
}

func TestFileRepo_FileToBaseName(t *testing.T) {
	r, err := filerepo.New("testdata", "join")
	require.NoError(t, err)

	filePath := filepath.Join("testdata", "join", "mychannel.join")
	channelName := r.FileToBaseName(filePath)
	require.Equal(t, "mychannel", channelName)
}
