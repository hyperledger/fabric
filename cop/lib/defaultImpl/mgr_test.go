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

package defaultImpl

import "testing"

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Error("failed to create new server")
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Error("failed to create new client")
	}
}

func TestNewCertMgr(t *testing.T) {
	cm := NewCertMgr()
	if cm == nil {
		t.Error("failed to create new cert mgr")
	}
}
