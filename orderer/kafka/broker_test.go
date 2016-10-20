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

package kafka

import "testing"

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestBrokerGetOffset(t *testing.T) {
	t.Run("oldest", testBrokerGetOffsetFunc(sarama.OffsetOldest, oldestOffset))
	t.Run("newest", testBrokerGetOffsetFunc(sarama.OffsetNewest, newestOffset))
} */

func testBrokerGetOffsetFunc(given, expected int64) func(t *testing.T) {
	return func(t *testing.T) {
		mb := mockNewBroker(t, testConf)
		defer testClose(t, mb)

		offset, _ := mb.GetOffset(newOffsetReq(mb.(*mockBrockerImpl).config, given))
		if offset != expected {
			t.Fatalf("Expected offset %d, got %d instead", expected, offset)
		}
	}
}
