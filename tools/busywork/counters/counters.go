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

// For user-level documentation of the operation and semantics of this
// chaincode, see the README.md file contained in this directory.

package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"

	busy "github.com/hyperledger/fabric/tools/busywork/busywork"
)

// counters implementation
//
// NB: It is not clear whether this usage of the stub, that is, storing the
// stub in the counters object, will work in future versions of the
// architecture that support concurrency. For now it is handy to put it here;
// in the future a different way of exposing the shim might be preferred.
type counters struct {
	logger *shim.ChaincodeLogger // Our logger
	id     string                // Chaincode ID
	stub   shim.ChaincodeStubInterface // The stub
}

// newCounters is a "constructor" for counters objects
func newCounters() *counters {
	c := new(counters)
	c.logger = shim.NewLogger("counters:<uninitialized>")
	return c
}

// debugf prints a debug message
func (c *counters) debugf(format string, args ...interface{}) {
	c.logger.Debugf(format, args...)
}

// infof prints an info message
func (c *counters) infof(format string, args ...interface{}) {
	c.logger.Infof(format, args...)
}

// errorf logs irrecoverable errors and "throws" a busywork panic
func (c *counters) errorf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	c.logger.Errorf(s)
	busy.Throw(s)
}

// criticalf logs critical irrecoverable errors and "throws" a busywork panic
func (c *counters) criticalf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	c.logger.Criticalf(s)
	busy.Throw(s)
}

// assert traps assertion failures (bugs) as panics
func (c *counters) assert(p bool, format string, args ...interface{}) {
	if !p {
		s := fmt.Sprintf("ASSERTION FAILURE:\n"+format, args...)
		fmt.Fprintf(os.Stderr, s)
		panic(s)
	}
}

// lengthKey computes the key used to store the array length
func lengthKey(name string) string {
	return "length" + "\x00" + name
}

// getUint64 is a generic GetState() for integers
func (c *counters) getUint64(name string) uint64 {
	b, err := c.stub.GetState(name)
	if err != nil {
		c.criticalf("getUint64: Error from getState(%s): %s", name, err)
	}
	if len(b) != 8 {
		c.criticalf("getUint64: Reading '%s', expected 8 bytes but read %d", name, len(b))
	}
	var i uint64
	err = binary.Read(bytes.NewReader(b), binary.LittleEndian, &i)
	if err != nil {
		c.criticalf("getUint64: Error converting bytes of '%s' to uint64: %s", name, err)
	}
	return i
}

// putUint64 is a generic PutState() for integers
func (c *counters) putUint64(name string, x uint64) {
	b := new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, x)
	if err != nil {
		c.criticalf("putUint64: Error from binary.Write(): %s", err)
	}
	err = c.stub.PutState(name, b.Bytes())
	if err != nil {
		c.criticalf("putuint64: Error from PutState(%s): %s", name, err)
	}
}

// getArray gets an array from the state and does a few basic consistency
// checks.
func (c *counters) getArray(name string) []uint64 {
	b, err := c.stub.GetState(name)
	if err != nil {
		c.criticalf("getArray: GetState() for array '%s' failed: %s", name, err)
	}
	if len(b) == 0 {
		c.criticalf("getArray: Array '%s' is empty", name)
	}
	if len(b)%8 != 0 {
		c.criticalf("getArray: Array '%s' was retreived as %d bytes; Expected a multiple of 8", name, len(b))
	}
	array := make([]uint64, len(b)/8)
	err = binary.Read(bytes.NewReader(b), binary.LittleEndian, &array)
	if err != nil {
		c.criticalf("getArray: Error converting array '%s' (length %d) to uint64: %s", name, len(b), err)
	}
	return array
}

// putArray puts an array back to the state
func (c *counters) putArray(name string, array []uint64) {
	b := new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, array)
	if err != nil {
		c.criticalf("putArray: Error on binary.Write() for array '%s': %s", name, err)
	}
	err = c.stub.PutState(name, b.Bytes())
	if err != nil {
		c.criticalf("putArray: Error on PutState() for array '%s': %s", name, err)
	}
}

// create (re-)creates one or more counter arrays and zeros their state.
func (c *counters) create(args []string) (val []byte, err error) {

	// There must always be an even number of argument strings, and the odd
	// (length) strings must parse as non-0 unsigned 64-bit values.

	if (len(args) % 2) != 0 {
		c.errorf("create: Odd number of parameters; Must be pairs of ... <name> <length> ...")
	}
	name := false
	var names []string
	var lengths []uint64
	for n, x := range args {
		if name = !name; name {
			c.debugf("create : Name = %s", x)
			names = append(names, x)
		} else {
			length, err := strconv.ParseUint(x, 0, 64)
			if err != nil {
				c.errorf("create: This - '%s' - does not parse as a 64-bit integer", x)
			}
			if length <= 0 {
				c.errorf("create: Argument %d is negative or 0", n)
			}
			c.debugf("create: Length = %d", length)
			lengths = append(lengths, length)
		}
	}

	// Now create and store each array. Note that we create the arrays
	// directly as byte arrays. The length of each array is stored in the
	// database.

	for n, name := range names {
		length := lengths[n]
		a := make([]byte, length*8)
		err := c.stub.PutState(name, a)
		if err != nil {
			c.criticalf("create: Error on PutState, array '%s' with length %d: %s", name, length, err)
		}
		c.putUint64(lengthKey(name), uint64(length))
	}

	return
}

// incDec either increments or decrements 0 or more counter arrays. The choice
// is made based on the value of 'incr'.
func (c *counters) incDec(args []string, incr int) (val []byte, err error) {

	c.assert((incr == 1) || (incr == -1), "The 'incr' parameter must be 1 or -1")

	// Check each array for existence and record the number of times it will
	// be incremented/decremented. We assume that no-one will be able to
	// process a command with >= 2^64 increments or decrements.

	offset := map[string]uint64{}
	var names []string
	for _, name := range args {
		if offset[name] == 0 {
			names = append(names, name)
		}
		offset[name]++
	}

	// Bring the arrays into (our) memory.

	arrays := map[string][]uint64{}
	for _, name := range names {
		arrays[name] = c.getArray(name)
	}

	// Increment/decrement, insuring that each array has consistent state in
	// every location. The unsigned offsets are "signed" here. Underflow and
	// overflow checks are performed on the first element here.

	for _, name := range names {

		counters := arrays[name]
		finalOffset := offset[name] * uint64(incr)
		expected := counters[0]

		if incr < 0 {
			if (expected + finalOffset) > expected {
				c.criticalf("incDec: Underflow on array '%s'", name)
			}
		} else {
			if (expected + finalOffset) < expected {
				c.criticalf("incDec: Overflow on array '%s'", name)
			}
		}

		for i, v := range counters {
			if v != expected {
				c.criticalf("incDec: Inconsistent state for element '%s[%d]', expecting %d, found %d", name, i, expected, v)
			}
			new := v + finalOffset
			c.debugf("incDec : %s[%d] <- %d", name, i, new)
			counters[i] = new
		}
	}

	// Write the arrays back to the state.

	for _, name := range names {
		c.putArray(name, arrays[name])
	}

	return
}

// initParms handles the initialization of `parms`.
func (c *counters) initParms(args []string) (val []byte, err error) {

	c.infof("initParms : Command-line arguments : %v", args)

	// Define and parse the parameters
	flags := flag.NewFlagSet("initParms", flag.ContinueOnError)
	flags.StringVar(&c.id, "id", "", "A unique identifier; Allows multiple copies of this chaincode to be deployed")
	loggingLevel := flags.String("loggingLevel", "default", "The logging level of the chaincode logger. Defaults to INFO.")
	shimLoggingLevel := flags.String("shimLoggingLevel", "default", "The logging level of the chaincode 'shim' interface. Defaults to WARNING.")
	err = flags.Parse(args)
	if err != nil {
		c.errorf("initParms : Error during option processing : %s", err)
	}

	// Replace the original logger, logging as "counters", with a new logger
	// logging as "counters:<id>", then set its logging level and the logging
	// level of the shim.
	c.logger = shim.NewLogger("counters:" + c.id)
	if *loggingLevel == "default" {
		c.logger.SetLevel(shim.LogInfo)
	} else {
		level, _ := shim.LogLevel(*loggingLevel)
		c.logger.SetLevel(level)
	}
	if *shimLoggingLevel != "default" {
		level, _ := shim.LogLevel(*shimLoggingLevel)
		shim.SetLoggingLevel(level)
	}
	return
}

// queryParms handles the `parms` query
func (c *counters) queryParms(args []string) (val []byte, err error) {
	flags := flag.NewFlagSet("queryParms", flag.ContinueOnError)
	flags.StringVar(&c.id, "id", "", "Uniquely identify a chaincode instance")
	err = flags.Parse(args)
	if err != nil {
		c.errorf("queryParms : Error during option processing : %s", err)
	}
	return
}

// status implements the `status` query.
func (c *counters) status(args []string) (val []byte, err error) {

	// Run down the list of arrays, pulling their state into our memory

	arrays := map[string][]uint64{}
	for _, name := range args {
		arrays[name] = c.getArray(name)
	}

	// Now create the result

	res := ""
	for _, name := range args {
		if res != "" {
			res += " "
		}
		expectedLength := c.getUint64(lengthKey(name))
		actualLength := uint64(len(arrays[name]))
		actualCount := arrays[name][0]
		expectedCount := actualCount
		res += fmt.Sprintf("%d %d %d %d", expectedLength, actualLength, expectedCount, actualCount)
	}
	c.debugf("status: Final status: %s", res)
	return []byte(res), nil
}

// Init handles chaincode initialization. Only the 'parms' function is
// recognized here.
func (c *counters) Init(stub shim.ChaincodeStubInterface, function string, args []string) (val []byte, err error) {
	c.stub = stub
	defer busy.Catch(&err)
	switch function {
	case "parms":
		return c.initParms(args)
	default:
		c.errorf("Init : Function '%s' is not recognized for INIT", function)
	}
	return
}

// Invoke handles the `invoke` methods.
func (c *counters) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) (val []byte, err error) {
	c.stub = stub
	defer busy.Catch(&err)
	switch function {
	case "create":
		return c.create(args)
	case "decrement":
		return c.incDec(args, -1)
	case "increment":
		return c.incDec(args, 1)
	default:
		c.errorf("Invoke : Function '%s' is not recognized for INVOKE", function)
	}
	return
}

// Query handles the `query` methods.
func (c *counters) Query(stub shim.ChaincodeStubInterface, function string, args []string) (val []byte, err error) {
	c.stub = stub
	defer busy.Catch(&err)
	switch function {
	case "parms":
		return c.queryParms(args)
	case "ping":
		return []byte(c.id), nil
	case "status":
		return c.status(args)
	default:
		c.errorf("Query : Function '%s' is not recognized for QUERY", function)
	}
	return
}

func main() {

	c := newCounters()

	c.logger.SetLevel(shim.LogInfo)
	shim.SetLoggingLevel(shim.LogWarning)

	// This is required because we need the lengths (len()) of our arrays to
	// support large (> 2^32) byte arrays.
	c.assert(busy.SizeOfInt() == 8, "The 'counters' chaincode is only supported on platforms where Go integers are 8-byte integers")

	err := shim.Start(c)
	if err != nil {
		c.logger.Criticalf("main : Error starting counters chaincode: %s", err)
		os.Exit(1)
	}
}
