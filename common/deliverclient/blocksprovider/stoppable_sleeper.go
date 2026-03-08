/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import "time"

//go:generate counterfeiter -o fake/sleeper.go --fake-name Sleeper . testSleeper
type customSleeper interface {
	Sleep(time.Duration)
}

type sleeper struct {
	sleep func(time.Duration)
}

func (s sleeper) Sleep(d time.Duration, doneC chan struct{}) {
	if s.sleep == nil {
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-doneC:
			timer.Stop()
		}
		return
	}
	s.sleep(d)
}
