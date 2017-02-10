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

package test

import (
	"testing"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestMakeGenesisBlock(t *testing.T) {
	_, err := MakeGenesisBlock("foo")
	if err != nil {
		t.Fatalf("Error making genesis block: %s", err)
	}
}

func TestOrdererTemplate(t *testing.T) {
	_ = OrdererTemplate()
}

func TestMSPTemplate(t *testing.T) {
	_ = MSPTemplate()
}

func TestApplicationTemplate(t *testing.T) {
	_ = ApplicationTemplate()
}

func TestCompositeTemplate(t *testing.T) {
	_ = CompositeTemplate()
}
