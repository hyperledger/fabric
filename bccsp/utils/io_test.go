/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDirExists(t *testing.T) {
	r, err := DirExists("")
	assert.False(t, r)
	assert.NoError(t, err)

	r, err = DirExists(os.TempDir())
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = DirExists(filepath.Join(os.TempDir(), "7rhf90239vhev90"))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

func TestDirMissingOrEmpty(t *testing.T) {
	r, err := DirMissingOrEmpty("")
	assert.NoError(t, err)
	assert.True(t, r)

	r, err = DirMissingOrEmpty(filepath.Join(os.TempDir(), "7rhf90239vhev90"))
	assert.NoError(t, err)
	assert.Equal(t, true, r)
}

func TestDirEmpty(t *testing.T) {
	_, err := DirEmpty("")
	assert.Error(t, err)

	path := filepath.Join(os.TempDir(), "7rhf90239vhev90")
	defer os.Remove(path)
	os.Mkdir(path, os.ModePerm)

	r, err := DirEmpty(path)
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = DirEmpty(os.TempDir())
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = DirMissingOrEmpty(os.TempDir())
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = DirMissingOrEmpty(path)
	assert.NoError(t, err)
	assert.Equal(t, true, r)
}
