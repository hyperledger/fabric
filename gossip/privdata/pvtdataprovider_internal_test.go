/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"time"
)

//go:generate counterfeiter -o mocks/sleeper.go --fake-name Sleeper . testSleeper
type testSleeper interface {
	Sleep(time.Duration)
}

func SetSleeper(pdp *PvtdataProvider, sleeper testSleeper) {
	pdp.sleeper.sleep = sleeper.Sleep
}
