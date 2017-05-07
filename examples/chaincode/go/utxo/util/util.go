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

package util

import (
	"bytes"
	"encoding/binary"
)

func decodeInt32(input []byte) (int32, []byte, error) {
	var myint32 int32
	buf1 := bytes.NewBuffer(input[0:4])
	binary.Read(buf1, binary.LittleEndian, &myint32)
	return myint32, input, nil
}

// ReadVarInt reads an int that is formatted in the Bitcoin style
// variable int format
func ReadVarInt(buffer *bytes.Buffer) uint64 {
	var finalResult uint64

	var variableLenInt uint8
	binary.Read(buffer, binary.LittleEndian, &variableLenInt)
	if variableLenInt < 253 {
		finalResult = uint64(variableLenInt)
	} else if variableLenInt == 253 {
		var result uint16
		binary.Read(buffer, binary.LittleEndian, &result)
		finalResult = uint64(result)
	} else if variableLenInt == 254 {
		var result uint32
		binary.Read(buffer, binary.LittleEndian, &result)
		finalResult = uint64(result)
	} else if variableLenInt == 255 {
		var result uint64
		binary.Read(buffer, binary.LittleEndian, &result)
		finalResult = result
	}

	return finalResult
}

// ParseUTXOBytes parses a bitcoin style transaction
func ParseUTXOBytes(txAsUTXOBytes []byte) *TX {
	buffer := bytes.NewBuffer(txAsUTXOBytes)
	var version int32
	binary.Read(buffer, binary.LittleEndian, &version)

	inputCount := ReadVarInt(buffer)

	newTX := &TX{}

	for i := 0; i < int(inputCount); i++ {
		newTXIN := &TX_TXIN{}

		newTXIN.SourceHash = buffer.Next(32)

		binary.Read(buffer, binary.LittleEndian, &newTXIN.Ix)

		// Parse the script length and script bytes
		scriptLength := ReadVarInt(buffer)
		newTXIN.Script = buffer.Next(int(scriptLength))

		// Appears to not be used currently
		binary.Read(buffer, binary.LittleEndian, &newTXIN.Sequence)

		newTX.Txin = append(newTX.Txin, newTXIN)

	}

	// Now the outputs
	outputCount := ReadVarInt(buffer)

	for i := 0; i < int(outputCount); i++ {
		newTXOUT := &TX_TXOUT{}

		binary.Read(buffer, binary.LittleEndian, &newTXOUT.Value)

		// Parse the script length and script bytes
		scriptLength := ReadVarInt(buffer)
		newTXOUT.Script = buffer.Next(int(scriptLength))

		newTX.Txout = append(newTX.Txout, newTXOUT)

	}

	binary.Read(buffer, binary.LittleEndian, &newTX.LockTime)

	return newTX
}
