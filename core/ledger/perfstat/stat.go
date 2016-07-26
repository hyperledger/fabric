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

package perfstat

import (
	"fmt"
	"sync"
	"time"
)

type stat struct {
	rwLock   sync.RWMutex
	statName string
	desc     string

	numInvocations int64
	total          int64
	min            int64
	max            int64
}

func newStat(name string, desc string) *stat {
	return &stat{statName: name, desc: desc, min: int64(^uint64(0) >> 1)}
}

func (s *stat) String() string {
	s.rwLock.RLock()
	defer s.rwLock.RUnlock()
	return fmt.Sprintf("%s: [total:%d, numInvocation:%d, average:%d, min=%d, max=%d]",
		s.statName, s.total, s.numInvocations, (s.total / s.numInvocations), s.min, s.max)
}

func (s *stat) reset() {
	s.rwLock.Lock()
	defer s.rwLock.Unlock()
	s.numInvocations = 0
	s.min = int64(^uint64(0) >> 1)
	s.max = 0
	s.total = 0
}

func (s *stat) updateTimeSpent(startTime time.Time) {
	s.updateDataStat(time.Since(startTime).Nanoseconds())
}

func (s *stat) updateDataStat(value int64) {
	s.rwLock.Lock()
	defer s.rwLock.Unlock()
	s.numInvocations++
	s.total += value
	if value < s.min {
		s.min = value
	} else if value > s.max {
		s.max = value
	}
}
