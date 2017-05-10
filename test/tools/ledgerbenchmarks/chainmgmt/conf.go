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

package chainmgmt

// ChainMgrConf captures the configurations meant at the level of chainMgr
// DataDir field specifies the filesystem location where the chains data is maintained
// NumChains field specifies the number of chains to instantiate
type ChainMgrConf struct {
	DataDir   string
	NumChains int
}

// BatchConf captures the batch related configurations
// BatchSize specifies the number of transactions in one block
// SignBlock specifies whether the transactions should be signed
type BatchConf struct {
	BatchSize int
	SignBlock bool
}
