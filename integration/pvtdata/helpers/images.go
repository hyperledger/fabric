/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/common/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	docker "github.com/fsouza/go-dockerclient"
)

func AssertImagesExist(imageNames ...string) {
	dockerClient, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	for _, imageName := range imageNames {
		images, err := dockerClient.ListImages(docker.ListImagesOptions{
			Filter: imageName,
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		if len(images) != 1 {
			Fail(fmt.Sprintf("missing required image: %s", imageName), 1)
		}
	}
}

// UniqueName generates base-32 enocded UUIDs for container names.
func UniqueName() string {
	name := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(util.GenerateBytesUUID())
	return strings.ToLower(name)
}
