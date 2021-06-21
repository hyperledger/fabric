// +build cgo

/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package y

import (
	"github.com/DataDog/zstd"
)

// CgoEnabled is used to check if CGO is enabled while building badger.
const CgoEnabled = true

// ZSTDDecompress decompresses a block using ZSTD algorithm.
func ZSTDDecompress(dst, src []byte) ([]byte, error) {
	return zstd.Decompress(dst, src)
}

// ZSTDCompress compresses a block using ZSTD algorithm.
func ZSTDCompress(dst, src []byte, compressionLevel int) ([]byte, error) {
	return zstd.CompressLevel(dst, src, compressionLevel)
}
