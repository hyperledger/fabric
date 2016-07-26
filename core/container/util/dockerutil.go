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

package util

import (
	"github.com/fsouza/go-dockerclient"
	"github.com/spf13/viper"
)

//NewDockerClient creates a docker client
func NewDockerClient() (client *docker.Client, err error) {
	endpoint := viper.GetString("vm.endpoint")
	tlsenabled := viper.GetBool("vm.docker.tls.enabled")
	if tlsenabled {
		cert := viper.GetString("vm.docker.tls.cert.file")
		key := viper.GetString("vm.docker.tls.key.file")
		ca := viper.GetString("vm.docker.tls.ca.file")
		client, err = docker.NewTLSClient(endpoint, cert, key, ca)
	} else {
		client, err = docker.NewClient(endpoint)
	}
	return
}
