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

package simplebft

import (
	"sort"
	"time"
)

type calendarQueue struct {
	dayLength  time.Duration
	yearLength time.Duration
	today      time.Duration
	nextYear   time.Duration
	slots      [][]testElem
	maxLen     int
}

func newCalendarQueue(dayLength time.Duration, days int) *calendarQueue {
	return &calendarQueue{
		dayLength:  dayLength,
		yearLength: dayLength * time.Duration(days),
		nextYear:   dayLength * time.Duration(days),
		slots:      make([][]testElem, days),
	}
}

func (t *calendarQueue) slot(d time.Duration) int {
	return int(d/t.dayLength) % len(t.slots)
}

func (t *calendarQueue) Add(e testElem) {
	sl := t.slot(e.at)
	t.slots[sl] = append(t.slots[sl], e)
	l := len(t.slots[sl])
	if l > t.maxLen {
		t.maxLen = l
	}
	sort.Sort(testElemQueue(t.slots[sl]))
}

func (t *calendarQueue) Pop() (testElem, bool) {
	var lowest *time.Duration
	sl := t.slot(t.today)
	start := sl
	today := t.today
	for ; today < t.nextYear; today, sl = today+t.dayLength, sl+1 {
		if len(t.slots[sl]) == 0 {
			continue
		}
		e := t.slots[sl][0]
		if e.at >= t.nextYear {
			if lowest == nil || *lowest > e.at {
				lowest = &e.at
			}
			continue
		}
		t.slots[sl] = t.slots[sl][1:]
		t.today = today
		return e, true
	}

	// next deadline is after this year, but we only
	// searched part of the calendar so far.  Search the
	// remaining prefix.
	for i := 0; i < start; i++ {
		if len(t.slots[i]) == 0 {
			continue
		}
		e := t.slots[i][0]
		if e.at >= t.nextYear {
			if lowest == nil || *lowest > e.at {
				lowest = &e.at
			}
		}
	}

	if lowest == nil {
		return testElem{}, false
	}

	t.today = *lowest / t.dayLength * t.dayLength
	t.nextYear = (t.today/t.yearLength + 1) * t.yearLength
	return t.Pop() // retry!
}

func (t *calendarQueue) filter(fn func(testElem) bool) {
	for sli, sl := range t.slots {
		var del []int
		for i, e := range sl {
			if !fn(e) {
				del = append(del, i)
			}
		}

		// now delete
		for i, e := range del {
			correctedPos := e - i
			// in-place remove
			sl = sl[:correctedPos+copy(sl[correctedPos:], sl[correctedPos+1:])]
		}
		t.slots[sli] = sl
	}
}

/////////////////////////////////////////

type testElemQueue []testElem

func (q testElemQueue) Len() int {
	return len(q)
}

func (q testElemQueue) Less(i, j int) bool {
	return q[i].at < q[j].at
}

func (q testElemQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}
