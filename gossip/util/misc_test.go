/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testHappyPath(t *testing.T) {
	n1 := RandomInt(10000)
	n2 := RandomInt(10000)
	require.NotEqual(t, n1, n2)
	n3 := RandomUInt64()
	n4 := RandomUInt64()
	require.NotEqual(t, n3, n4)
}

func TestContains(t *testing.T) {
	require.True(t, Contains("foo", []string{"bar", "foo", "baz"}))
	require.False(t, Contains("foo", []string{"bar", "baz"}))
}

func TestGetRandomInt(t *testing.T) {
	testHappyPath(t)
}

func TestNonNegativeValues(t *testing.T) {
	require.True(t, RandomInt(1000000) >= 0)
}

func TestGetRandomIntBadInput(t *testing.T) {
	f1 := func() {
		RandomInt(0)
	}
	f2 := func() {
		RandomInt(-500)
	}
	require.Panics(t, f1)
	require.Panics(t, f2)
}

type reader struct {
	mock.Mock
}

func (r *reader) Read(p []byte) (int, error) {
	args := r.Mock.Called(p)
	n := args.Get(0).(int)
	err := args.Get(1)
	if err == nil {
		return n, nil
	}
	return n, err.(error)
}

func TestGetRandomIntNoEntropy(t *testing.T) {
	rr := rand.Reader
	defer func() {
		rand.Reader = rr
	}()
	r := &reader{}
	r.On("Read", mock.Anything).Return(0, errors.New("Not enough entropy"))
	rand.Reader = r
	// Make sure randomness still works even when we have no entropy
	testHappyPath(t)
}

func TestRandomIndices(t *testing.T) {
	// not enough choices as needed
	require.Nil(t, GetRandomIndices(10, 5))
	// exact number of choices as available
	require.Len(t, GetRandomIndices(10, 9), 10)
	// more choices available than needed
	require.Len(t, GetRandomIndices(10, 90), 10)
}

func TestGetIntOrDefault(t *testing.T) {
	viper.Set("N", 100)
	n := GetIntOrDefault("N", 100)
	require.Equal(t, 100, n)
	m := GetIntOrDefault("M", 101)
	require.Equal(t, 101, m)
}

func TestGetDurationOrDefault(t *testing.T) {
	viper.Set("foo", time.Second)
	foo := GetDurationOrDefault("foo", time.Second*2)
	require.Equal(t, time.Second, foo)
	bar := GetDurationOrDefault("bar", time.Second*2)
	require.Equal(t, time.Second*2, bar)
}

func TestPrintStackTrace(t *testing.T) {
	PrintStackTrace()
}

func TestGetLogger(t *testing.T) {
	l1 := GetLogger("foo", "bar")
	l2 := GetLogger("foo", "bar")
	require.Equal(t, l1, l2)
}

func TestSet(t *testing.T) {
	s := NewSet()
	require.Len(t, s.ToArray(), 0)
	require.Equal(t, s.Size(), 0)
	require.False(t, s.Exists(42))
	s.Add(42)
	require.True(t, s.Exists(42))
	require.Len(t, s.ToArray(), 1)
	require.Equal(t, s.Size(), 1)
	s.Remove(42)
	require.False(t, s.Exists(42))
	s.Add(42)
	require.True(t, s.Exists(42))
	s.Clear()
	require.False(t, s.Exists(42))
}

func TestStringsToBytesToStrings(t *testing.T) {
	strings := []string{"foo", "bar"}
	require.Equal(t, strings, BytesToStrings(StringsToBytes(strings)))
}
