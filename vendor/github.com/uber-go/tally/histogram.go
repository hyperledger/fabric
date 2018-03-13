// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tally

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

var (
	// DefaultBuckets can be passed to specify to default buckets.
	DefaultBuckets Buckets

	errBucketsCountNeedsGreaterThanZero = errors.New("n needs to be > 0")
	errBucketsStartNeedsGreaterThanZero = errors.New("start needs to be > 0")
	errBucketsFactorNeedsGreaterThanOne = errors.New("factor needs to be > 1")
)

// ValueBuckets is a set of float64 values that implements Buckets.
type ValueBuckets []float64

// Implements sort.Interface
func (v ValueBuckets) Len() int {
	return len(v)
}

// Implements sort.Interface
func (v ValueBuckets) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// Implements sort.Interface
func (v ValueBuckets) Less(i, j int) bool {
	return v[i] < v[j]
}

func (v ValueBuckets) String() string {
	values := make([]string, len(v))
	for i := range values {
		values[i] = fmt.Sprintf("%f", v[i])
	}
	return fmt.Sprint(values)
}

// AsValues implements Buckets.
func (v ValueBuckets) AsValues() []float64 {
	return []float64(v)
}

// AsDurations implements Buckets and returns time.Duration
// representations of the float64 values divided by time.Second.
func (v ValueBuckets) AsDurations() []time.Duration {
	values := make([]time.Duration, len(v))
	for i := range values {
		values[i] = time.Duration(v[i] * float64(time.Second))
	}
	return values
}

// DurationBuckets is a set of time.Duration values that implements Buckets.
type DurationBuckets []time.Duration

// Implements sort.Interface
func (v DurationBuckets) Len() int {
	return len(v)
}

// Implements sort.Interface
func (v DurationBuckets) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// Implements sort.Interface
func (v DurationBuckets) Less(i, j int) bool {
	return v[i] < v[j]
}

func (v DurationBuckets) String() string {
	values := make([]string, len(v))
	for i := range values {
		values[i] = v[i].String()
	}
	return fmt.Sprintf("%v", values)
}

// AsValues implements Buckets and returns float64
// representations of the time.Duration values divided by time.Second.
func (v DurationBuckets) AsValues() []float64 {
	values := make([]float64, len(v))
	for i := range values {
		values[i] = float64(v[i]) / float64(time.Second)
	}
	return values
}

// AsDurations implements Buckets.
func (v DurationBuckets) AsDurations() []time.Duration {
	return []time.Duration(v)
}

// BucketPairs creates a set of bucket pairs from a set
// of buckets describing the lower and upper bounds for
// each derived bucket.
func BucketPairs(buckets Buckets) []BucketPair {
	if buckets == nil || buckets.Len() < 1 {
		return []BucketPair{

			bucketPair{
				lowerBoundValue:    -math.MaxFloat64,
				upperBoundValue:    math.MaxFloat64,
				lowerBoundDuration: time.Duration(math.MinInt64),
				upperBoundDuration: time.Duration(math.MaxInt64),
			},
		}
	}

	var (
		asValueBuckets    = copyAndSortValues(buckets.AsValues())
		asDurationBuckets = copyAndSortDurations(buckets.AsDurations())
		pairs             = make([]BucketPair, 0, buckets.Len()+2)
	)

	pairs = append(pairs, bucketPair{
		lowerBoundValue:    -math.MaxFloat64,
		upperBoundValue:    asValueBuckets[0],
		lowerBoundDuration: time.Duration(math.MinInt64),
		upperBoundDuration: asDurationBuckets[0],
	})

	prevValueBucket, prevDurationBucket :=
		asValueBuckets[0], asDurationBuckets[0]

	for i := 1; i < buckets.Len(); i++ {
		pairs = append(pairs, bucketPair{
			lowerBoundValue:    prevValueBucket,
			upperBoundValue:    asValueBuckets[i],
			lowerBoundDuration: prevDurationBucket,
			upperBoundDuration: asDurationBuckets[i],
		})
		prevValueBucket, prevDurationBucket =
			asValueBuckets[i], asDurationBuckets[i]
	}

	pairs = append(pairs, bucketPair{
		lowerBoundValue:    prevValueBucket,
		upperBoundValue:    math.MaxFloat64,
		lowerBoundDuration: prevDurationBucket,
		upperBoundDuration: time.Duration(math.MaxInt64),
	})

	return pairs
}

func copyAndSortValues(values []float64) []float64 {
	valuesCopy := make([]float64, len(values))
	for i := range values {
		valuesCopy[i] = values[i]
	}
	sort.Sort(ValueBuckets(valuesCopy))
	return valuesCopy
}

func copyAndSortDurations(durations []time.Duration) []time.Duration {
	durationsCopy := make([]time.Duration, len(durations))
	for i := range durations {
		durationsCopy[i] = durations[i]
	}
	sort.Sort(DurationBuckets(durationsCopy))
	return durationsCopy
}

type bucketPair struct {
	lowerBoundValue    float64
	upperBoundValue    float64
	lowerBoundDuration time.Duration
	upperBoundDuration time.Duration
}

func (p bucketPair) LowerBoundValue() float64 {
	return p.lowerBoundValue
}

func (p bucketPair) UpperBoundValue() float64 {
	return p.upperBoundValue
}

func (p bucketPair) LowerBoundDuration() time.Duration {
	return p.lowerBoundDuration
}

func (p bucketPair) UpperBoundDuration() time.Duration {
	return p.upperBoundDuration
}

// LinearValueBuckets creates a set of linear value buckets.
func LinearValueBuckets(start, width float64, n int) (ValueBuckets, error) {
	if n <= 0 {
		return nil, errBucketsCountNeedsGreaterThanZero
	}
	buckets := make([]float64, n)
	for i := range buckets {
		buckets[i] = start + (float64(i) * width)
	}
	return ValueBuckets(buckets), nil
}

// MustMakeLinearValueBuckets creates a set of linear value buckets
// or panics.
func MustMakeLinearValueBuckets(start, width float64, n int) ValueBuckets {
	buckets, err := LinearValueBuckets(start, width, n)
	if err != nil {
		panic(err)
	}
	return buckets
}

// LinearDurationBuckets creates a set of linear duration buckets.
func LinearDurationBuckets(start, width time.Duration, n int) (DurationBuckets, error) {
	if n <= 0 {
		return nil, errBucketsCountNeedsGreaterThanZero
	}
	buckets := make([]time.Duration, n)
	for i := range buckets {
		buckets[i] = start + (time.Duration(i) * width)
	}
	return DurationBuckets(buckets), nil
}

// MustMakeLinearDurationBuckets creates a set of linear duration buckets.
// or panics.
func MustMakeLinearDurationBuckets(start, width time.Duration, n int) DurationBuckets {
	buckets, err := LinearDurationBuckets(start, width, n)
	if err != nil {
		panic(err)
	}
	return buckets
}

// ExponentialValueBuckets creates a set of exponential value buckets.
func ExponentialValueBuckets(start, factor float64, n int) (ValueBuckets, error) {
	if n <= 0 {
		return nil, errBucketsCountNeedsGreaterThanZero
	}
	if start <= 0 {
		return nil, errBucketsStartNeedsGreaterThanZero
	}
	if factor <= 1 {
		return nil, errBucketsFactorNeedsGreaterThanOne
	}
	buckets := make([]float64, n)
	curr := start
	for i := range buckets {
		buckets[i] = curr
		curr *= factor
	}
	return ValueBuckets(buckets), nil
}

// MustMakeExponentialValueBuckets creates a set of exponential value buckets
// or panics.
func MustMakeExponentialValueBuckets(start, factor float64, n int) ValueBuckets {
	buckets, err := ExponentialValueBuckets(start, factor, n)
	if err != nil {
		panic(err)
	}
	return buckets
}

// ExponentialDurationBuckets creates a set of exponential duration buckets.
func ExponentialDurationBuckets(start time.Duration, factor float64, n int) (DurationBuckets, error) {
	if n <= 0 {
		return nil, errBucketsCountNeedsGreaterThanZero
	}
	if start <= 0 {
		return nil, errBucketsStartNeedsGreaterThanZero
	}
	if factor <= 1 {
		return nil, errBucketsFactorNeedsGreaterThanOne
	}
	buckets := make([]time.Duration, n)
	curr := start
	for i := range buckets {
		buckets[i] = curr
		curr = time.Duration(float64(curr) * factor)
	}
	return DurationBuckets(buckets), nil
}

// MustMakeExponentialDurationBuckets creates a set of exponential value buckets
// or panics.
func MustMakeExponentialDurationBuckets(start time.Duration, factor float64, n int) DurationBuckets {
	buckets, err := ExponentialDurationBuckets(start, factor, n)
	if err != nil {
		panic(err)
	}
	return buckets
}
