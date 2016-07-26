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

package ca

import (
	"reflect"
	"testing"

	pb "github.com/hyperledger/fabric/membersrvc/protos"
)

func TestRandomString(t *testing.T) {

	rand := randomString(100)
	if rand == "" {
		t.Error("The randomString function must return a non nil value.")
	}

	if reflect.TypeOf(rand).String() != "string" {
		t.Error("The randomString function must returns a string type value.")
	}
}

func TestMemberRoleToStringNone(t *testing.T) {

	var role, err = MemberRoleToString(pb.Role_NONE)

	if err != nil {
		t.Errorf("Error converting member role to string, result: %s", role)
	}

	if role != pb.Role_name[int32(pb.Role_NONE)] {
		t.Errorf("The role string returned should have been %s", "NONE")
	}

}
