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

package inspector

import (
	"fmt"
	cb "github.com/hyperledger/fabric/protos/common"
)

var typeMap map[cb.ConfigurationItem_ConfigurationType]ConfigurationItemValueLens = make(map[cb.ConfigurationItem_ConfigurationType]ConfigurationItemValueLens)

type ConfigurationItemValueLens interface {
	// Value takes a config item and returns a Viewable version of its value
	Value(configItem *cb.ConfigurationItem) Viewable
}

type Viewable interface {
	Value() string
	Children() []Viewable
}

type field struct {
	name   string
	values []Viewable
}

func (f *field) Value() string {
	return fmt.Sprintf("%s:", f.name)
}

func (f *field) Children() []Viewable {
	return f.values
}

const indent = 4

func printViewable(viewable Viewable, curDepth int) {
	fmt.Printf(fmt.Sprintf("%%%ds%%s\n", curDepth*indent), "", viewable.Value())
	for _, child := range viewable.Children() {
		printViewable(child, curDepth+1)
	}
}

func PrintConfiguration(configEnvelope *cb.ConfigurationEnvelope) {
	viewable := viewableConfigurationEnvelope("ConfigurationEnvelope", configEnvelope)
	printViewable(viewable, 0)
}
