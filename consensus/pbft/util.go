/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package pbft

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
)

func hash(msg interface{}) string {
	var raw []byte
	switch converted := msg.(type) {
	case *Request:
		raw, _ = proto.Marshal(converted)
	case *RequestBatch:
		raw, _ = proto.Marshal(converted)
	default:
		logger.Error("Asked to hash non-supported message type, ignoring")
		return ""
	}
	return base64.StdEncoding.EncodeToString(util.ComputeCryptoHash(raw))

}
