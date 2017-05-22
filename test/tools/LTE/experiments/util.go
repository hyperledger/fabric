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

package experiments

import (
	"bytes"
	"fmt"
	"math/rand"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("experiments")

func constructKey(keyNumber int) string {
	return fmt.Sprintf("%s%09d", "key_", keyNumber)
}

func constructValue(keyNumber int, kvSize int) []byte {
	prefix := constructValuePrefix(keyNumber)
	randomBytes := constructRandomBytes(kvSize - len(prefix))
	return append(prefix, randomBytes...)
}

func constructValuePrefix(keyNumber int) []byte {
	return []byte(fmt.Sprintf("%s%09d", "value_", keyNumber))
}

func verifyValue(keyNumber int, value []byte) bool {
	prefix := constructValuePrefix(keyNumber)
	if len(value) < len(prefix) {
		return false
	}
	return bytes.Equal(value[:len(prefix)], prefix)
}

func disableLogging() {
	logging.SetLevel(logging.ERROR, "")
}

func calculateShare(total int, numParts int, partNum int) int {
	share := total / numParts
	remainder := total % numParts
	if partNum < remainder {
		share++
	}
	return share
}

func constructRandomBytes(length int) []byte {
	b := make([]byte, length)
	rand.Read(b)
	return b
}
