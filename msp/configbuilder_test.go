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

package msp

import (
	"testing"

	"github.com/hyperledger/fabric/core/config"
	"github.com/stretchr/testify/assert"
)

func TestGetLocalMspConfig(t *testing.T) {
	mspDir, err := config.GetDevMspDir()
	assert.NoError(t, err)
	_, err = GetLocalMspConfig(mspDir, nil, "DEFAULT")
	assert.NoError(t, err)
}

func TestGetLocalMspConfigFails(t *testing.T) {
	_, err := GetLocalMspConfig("/tmp/", nil, "DEFAULT")
	assert.Error(t, err)
}

func TestReadFileUtils(t *testing.T) {
	// test that reading a file with an empty path doesn't crash
	_, err := readPemFile("")
	assert.Error(t, err)

	// test that reading an existing file which is not a PEM file doesn't crash
	_, err = readPemFile("/dev/null")
	assert.Error(t, err)
}
