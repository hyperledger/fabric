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

package main

import (
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

const (
	totalRows = 1000
)

type largeRowsetChaincode struct {
}

func (lrc *largeRowsetChaincode) retInAdd(ok bool, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("operation failed. %s", err)
	}
	if !ok {
		return nil, err
	}
	return nil, nil
}

// Init called for initializing the chaincode
func (lrc *largeRowsetChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	// Create a new table
	if err := stub.CreateTable("LargeTable", []*shim.ColumnDefinition{
		{Name: "Key", Type: shim.ColumnDefinition_STRING, Key: true},
		{"Name", shim.ColumnDefinition_STRING, false},
		{"Value", shim.ColumnDefinition_STRING, false},
	}); err != nil {
		//just assume the table exists and was populated
		return nil, nil
	}

	for i := 0; i < totalRows; i++ {
		col1 := fmt.Sprintf("Key_%d", i)
		col2 := fmt.Sprintf("Name_%d", i)
		col3 := fmt.Sprintf("Value_%d", i)
		if _, err := lrc.retInAdd(stub.InsertRow("LargeTable", shim.Row{Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: col1}},
			&shim.Column{Value: &shim.Column_String_{String_: col2}},
			&shim.Column{Value: &shim.Column_String_{String_: col3}},
		}})); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// Run callback representing the invocation of a chaincode
func (lrc *largeRowsetChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return nil, nil
}

// Query callback representing the query of a chaincode.
func (lrc *largeRowsetChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	var err error
	//stop at 1 greater than total rows by default (ie, read all rows)
	var stopAtRow = totalRows

	if len(args) > 0 {
		stopAtRow, err = strconv.Atoi(string(args[0]))
		if err != nil {
			return nil, err
		}
	}
	model := "LargeTable"

	var rowChannel <-chan shim.Row

	rowChannel, err = stub.GetRows(model, []shim.Column{})
	if err != nil {
		return nil, err
	}

	i := stopAtRow

	var rows []*shim.Row
	for {
		select {
		case row, ok := <-rowChannel:
			if !ok {
				rowChannel = nil
			} else {
				rows = append(rows, &row)
			}
		}

		i = i - 1
		if rowChannel == nil || i == 0 {
			break
		}
	}

	col1 := shim.Column{Value: &shim.Column_String_{String_: "Key_2"}}
	_, err = stub.GetRow(model, []shim.Column{col1})
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("%d", len(rows))), nil
}

func main() {
	err := shim.Start(new(largeRowsetChaincode))
	if err != nil {
		fmt.Printf("Error starting the chaincode: %s", err)
	}
}
