/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"time"

	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
)

const (
	runnerExecutionTimeout = time.Second * 30
)

func Execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	Eventually(p.Ready()).Should(BeClosed())
	Eventually(p.Wait(), runnerExecutionTimeout).Should(Receive(&err))
	return err
}

func Contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
