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
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("experiments")

type marbleRecord struct {
	ID          string `json:"_id,omitempty"`
	Rev         string `json:"_rev,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	AssetType   string `json:"asset_type,omitempty"`
	AssetName   string `json:"asset_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Size        int    `json:"size,omitempty"`
	Owner       string `json:"owner,omitempty"`
	DataPadding string `json:"datapadding,omitempty"`
}

var colors = []string{
	"red",
	"green",
	"purple",
	"yellow",
	"white",
	"black",
}

var owners = []string{
	"fred",
	"jerry",
	"tom",
	"alice",
	"kim",
	"angela",
	"john",
}

//TestValue is a struct for holding the test value
type TestValue struct {
	Value string
}

func constructKey(keyNumber int) string {
	return fmt.Sprintf("%s%09d", "key_", keyNumber)
}

func constructValue(keyNumber int, kvSize int) []byte {
	prefix := constructValuePrefix(keyNumber)
	randomBytes := constructRandomBytes(kvSize - len(prefix))

	return append(prefix, randomBytes...)
}

func constructJSONValue(keyNumber int, kvSize int) []byte {

	prefix := constructValuePrefix(keyNumber)

	rand.Seed(int64(keyNumber))
	color := colors[rand.Intn(len(colors))]
	size := rand.Intn(len(colors))*10 + 10
	owner := owners[rand.Intn(len(owners))]
	assetName := "marble" + strconv.Itoa(keyNumber)

	testRecord := marbleRecord{Prefix: string(prefix), AssetType: "marble", AssetName: assetName, Color: color, Size: size, Owner: owner}

	jsonValue, _ := json.Marshal(testRecord)

	if kvSize > len(jsonValue) {
		randomJSONBytes := constructRandomBytes(kvSize - len(jsonValue))

		//add in extra bytes
		testRecord.DataPadding = string(randomJSONBytes)

		jsonValue, _ = json.Marshal(testRecord)
	}

	return jsonValue

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

func verifyJSONValue(keyNumber int, value []byte) bool {
	prefix := constructValuePrefix(keyNumber)
	if len(value) < len(prefix) {
		return false
	}

	var marble marbleRecord

	json.Unmarshal(value, &marble)

	if len(value) < len(prefix) {
		return false
	}

	valuePrefix := []byte(marble.Prefix)
	return bytes.Equal(valuePrefix, prefix)
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
