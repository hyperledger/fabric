/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package fsblkstorage

import "path/filepath"

const (
	// ChainsDir is the name of the directory containing the channel ledgers.
	ChainsDir = "chains"
	// IndexDir is the name of the directory containing all block indexes across ledgers.
	IndexDir                = "index"
	defaultMaxBlockfileSize = 64 * 1024 * 1024 // bytes
)

// Conf encapsulates all the configurations for `FsBlockStore`
type Conf struct {
	blockStorageDir  string
	maxBlockfileSize int
}

// NewConf constructs new `Conf`.
// blockStorageDir is the top level folder under which `FsBlockStore` manages its data
func NewConf(blockStorageDir string, maxBlockfileSize int) *Conf {
	if maxBlockfileSize <= 0 {
		maxBlockfileSize = defaultMaxBlockfileSize
	}
	return &Conf{blockStorageDir, maxBlockfileSize}
}

func (conf *Conf) getIndexDir() string {
	return filepath.Join(conf.blockStorageDir, IndexDir)
}

func (conf *Conf) getChainsDir() string {
	return filepath.Join(conf.blockStorageDir, ChainsDir)
}

func (conf *Conf) getLedgerBlockDir(ledgerid string) string {
	return filepath.Join(conf.getChainsDir(), ledgerid)
}
