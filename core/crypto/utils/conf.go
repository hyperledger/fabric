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

package utils

import (
	"fmt"

	"github.com/spf13/viper"
)

// NodeConfiguration used for testing
type NodeConfiguration struct {
	Type string
	Name string
}

// GetEnrollmentID returns the enrollment ID
func (conf *NodeConfiguration) GetEnrollmentID() string {
	key := "tests.crypto.users." + conf.Name + ".enrollid"
	value := viper.GetString(key)
	if value == "" {
		panic(fmt.Errorf("Enrollment id not specified in configuration file. Please check that property '%s' is set", key))
	}
	return value
}

// GetEnrollmentPWD returns the enrollment PWD
func (conf *NodeConfiguration) GetEnrollmentPWD() string {
	key := "tests.crypto.users." + conf.Name + ".enrollpw"
	value := viper.GetString(key)
	if value == "" {
		panic(fmt.Errorf("Enrollment id not specified in configuration file. Please check that property '%s' is set", key))
	}
	return value
}
