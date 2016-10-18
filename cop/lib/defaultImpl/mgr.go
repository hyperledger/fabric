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

import (
	cop "github.com/hyperledger/fabric/cop/api"
)

// Mgr is the default COP manager
type Mgr struct {
}

// NewMgr is the constructor for the default COP manager
func NewMgr() (cop.Mgr, cop.Error) {
	mgr := new(Mgr)
	return mgr, nil
}

// GetProvider returns the provider name of this COP implementation
func (m *Mgr) GetProvider() string {
	return "default"
}

// GetFeatures returns the features of this COP implementation
func (m *Mgr) GetFeatures() []string {
	return []string{"server", "ecerts", "tcerts"}
}

// NewClient creates a new COP client
func (m *Mgr) NewClient() cop.Client {
	return NewClient()
}

// NewCertMgr creates a certificate handler
func (m *Mgr) NewCertMgr() cop.CertMgr {
	return new(CertMgr)
}
