/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runner Suite")
}

func ContainerExists(client *docker.Client, name string) func() bool {
	return func() bool {
		_, err := client.InspectContainer(name)
		if err != nil {
			_, ok := err.(*docker.NoSuchContainer)
			return !ok
		}
		return false
	}
}
