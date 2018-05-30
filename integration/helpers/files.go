/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"io/ioutil"

	. "github.com/onsi/gomega"
)

func CopyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0775)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
