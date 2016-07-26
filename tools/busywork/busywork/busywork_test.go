/*
Copyright IBM Corp. 2016. All Rights Reserved.

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

// Simple tests for the busywork package itself + other exercises

package busywork

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	//"crypto/rand"
	"time"
)

// TestOne(1) : Test that Throw/Catch works

func test1() (err error) {
	defer Catch(&err)
	Throw("irrecoverable")
	return
}

func TestOne(t *testing.T) {
	fmt.Println("IrrecoverableError panic should be caught")
	err := test1()
	if err == nil {
		t.Error("err is NIL")
	} else if err.Error() != "irrecoverable" {
		t.Errorf("After test1(), err.Error() is %s", err.Error())
	}
}

// TestTwo(2) : Test that Catch rethrows unrecognized panics

func test2() (err error) {
	defer Catch(&err)
	panic("unknown")
}

func TestTwo(t *testing.T) {
	fmt.Println("Unknown panic should be passed through, then caught")
	defer func() {
		p := recover()
		switch p.(type) {
		case nil:
			t.Error("No panic?")
		case IrrecoverableError:
			t.Error("IrecoverableError panic caught?")
		default:
		}
	}()

	test2()
	t.Error("We should have panicked")
}

// TestConversion : Test byte/uint64 array conversion methods

func TestConversion(t *testing.T) {
	fmt.Println("Testing byte/uint64 conversion")
	a := [4]uint64{0x0001020304050607, 0x0, 0x1, 0x8000000000000001}
	b := new(bytes.Buffer)
	err := binary.Write(b, binary.BigEndian, a)
	if err != nil {
		t.Errorf("Error on binary.Write : %s", err)
	}
	c := b.Bytes()
	if len(c) != 32 {
		t.Errorf("c : Expecting 32 bytes, got %d", len(c))
	}
	for i, x := range c {
		var mismatch bool
		if i < 8 {
			mismatch = (x != byte(i))
		} else if i < 23 {
			mismatch = (x != 0)
		} else if i == 23 {
			mismatch = (x != 1)
		} else if i == 24 {
			mismatch = (x != 0x80)
		} else if i == 31 {
			mismatch = (x != 1)
		} else {
			mismatch = (x != 0)
		}
		if mismatch {
			t.Errorf("mismatch at byte %d : %d", i, x)
		}
	}
	d := bytes.NewReader(c)
	var e [4]uint64
	err = binary.Read(d, binary.BigEndian, &e)
	if err != nil {
		t.Errorf("Error on binary.Read : %s", err)
	}
	if a != e {
		t.Error("Mismatch after reconstruction")
	}
}

// Test the size of int

func TestSizeofInt(t *testing.T) {
	fmt.Printf("Integers have %d bytes\n", SizeOfInt())
}

// Test latency of null timing

func TestNullTiming(t *testing.T) {
	n := 1000000
	var d time.Duration
	for i := 0; i < n; i++ {
		d = d + time.Since(time.Now())
	}
	fmt.Printf("A null call for an interval time takes %.1f nanoseconds\n", (d.Seconds()/float64(n))*1e9)
}
