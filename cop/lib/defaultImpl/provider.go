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

// Provider is the default COP provider implementation
type Provider struct {
}

// GetName returns the provider name
func (p *Provider) GetName() string {
	return "default"
}

// NewMgr instantiates a new instance of this mgr
func (p *Provider) NewMgr() (cop.Mgr, cop.Error) {
	mgr := new(Mgr)
	return mgr, nil
}
