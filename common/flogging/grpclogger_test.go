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

package flogging

import (
	"testing"

	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/grpclog"
)

// from go-logging memory_test.go
func MemoryRecordN(b *logging.MemoryBackend, n int) *logging.Record {
	node := b.Head()
	for i := 0; i < n; i++ {
		if node == nil {
			break
		}
		node = node.Next()
	}
	if node == nil {
		return nil
	}
	return node.Record
}

func TestGRPCLogger(t *testing.T) {
	initgrpclogger()
	backend := logging.NewMemoryBackend(3)
	logging.SetBackend(backend)
	logging.SetLevel(defaultLevel, "")
	SetModuleLevel(GRPCModuleID, "DEBUG")
	messages := []string{"print test", "printf test", "println test"}
	grpclog.Print(messages[0])
	grpclog.Printf(messages[1])
	grpclog.Println(messages[2])

	for i, message := range messages {
		assert.Equal(t, message, MemoryRecordN(backend, i).Message())
		t.Log(MemoryRecordN(backend, i).Message())
	}

	// now make sure there's no logging at a level other than DEBUG
	SetModuleLevel(GRPCModuleID, "INFO")
	messages2 := []string{"print test2", "printf test2", "println test2"}
	grpclog.Print(messages2[0])
	grpclog.Printf(messages2[1])
	grpclog.Println(messages2[2])

	// should still be messages not messages2
	for i, message := range messages {
		assert.Equal(t, message, MemoryRecordN(backend, i).Message())
		t.Log(MemoryRecordN(backend, i).Message())
	}
	// reset flogging
	Reset()
}
