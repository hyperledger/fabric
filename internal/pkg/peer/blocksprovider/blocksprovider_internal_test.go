/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"time"
)

//go:generate counterfeiter -o fake/sleeper.go --fake-name Sleeper . testSleeper
type testSleeper interface {
	Sleep(time.Duration)
}

func SetSleeper(d *Deliverer, sleeper testSleeper) {
	d.sleeper.sleep = sleeper.Sleep
}
