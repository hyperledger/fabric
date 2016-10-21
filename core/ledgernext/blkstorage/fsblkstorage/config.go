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

import "strings"

const (
	defaultMaxBlockfileSize = 64 * 1024 * 1024
)

// Conf encapsulates all the configurations for `FsBlockStore`
type Conf struct {
	blockfilesDir    string
	dbPath           string
	maxBlockfileSize int
}

// NewConf constructs new `Conf`.
// filesystemPath is the top level folder under which `FsBlockStore` manages its data
func NewConf(filesystemPath string, maxBlockfileSize int) *Conf {
	if !strings.HasSuffix(filesystemPath, "/") {
		filesystemPath = filesystemPath + "/"
	}
	if maxBlockfileSize <= 0 {
		maxBlockfileSize = defaultMaxBlockfileSize
	}
	return &Conf{filesystemPath + "blocks", filesystemPath + "db", maxBlockfileSize}
}
