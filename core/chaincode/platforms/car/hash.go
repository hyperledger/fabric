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

package car

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

//generateHashcode gets hashcode of the code under path. If path is a HTTP(s) url
//it downloads the code first to compute the hash.
//NOTE: for dev mode, user builds and runs chaincode manually. The name provided
//by the user is equivalent to the path. This method will treat the name
//as codebytes and compute the hash from it. ie, user cannot run the chaincode
//with the same (name, ctor, args)
func generateHashcode(spec *pb.ChaincodeSpec, path string) (string, error) {

	ctor := spec.CtorMsg
	if ctor == nil || len(ctor.Args) == 0 {
		return "", fmt.Errorf("Cannot generate hashcode from empty ctor")
	}
	ctorbytes, err := proto.Marshal(ctor)
	if err != nil {
		return "", fmt.Errorf("Error marshalling constructor: %s", err)
	}
	hash := util.GenerateHashFromSignature(spec.ChaincodeID.Path, ctorbytes)

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("Error reading file: %s", err)
	}

	newSlice := make([]byte, len(hash)+len(buf))
	copy(newSlice[len(buf):], hash[:])
	//hash = md5.Sum(newSlice)
	hash = util.ComputeCryptoHash(newSlice)

	return hex.EncodeToString(hash[:]), nil
}
