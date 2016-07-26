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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init create tables for tests
func (t *SimpleChaincode) Init(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	// Create table one
	err := createTableOne(stub)
	if err != nil {
		return nil, fmt.Errorf("Error creating table one during init. %s", err)
	}

	// Create table two
	err = createTableTwo(stub)
	if err != nil {
		return nil, fmt.Errorf("Error creating table two during init. %s", err)
	}

	// Create table three
	err = createTableThree(stub)
	if err != nil {
		return nil, fmt.Errorf("Error creating table three during init. %s", err)
	}

	// Create table four
	err = createTableFour(stub)
	if err != nil {
		return nil, fmt.Errorf("Error creating table four during init. %s", err)
	}

	return nil, nil
}

// Invoke callback representing the invocation of a chaincode
// This chaincode will manage two accounts A and B and will transfer X units from A to B upon invoke
func (t *SimpleChaincode) Invoke(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	switch function {

	case "insertRowTableOne":
		if len(args) < 3 {
			return nil, errors.New("insertTableOne failed. Must include 3 column values")
		}

		col1Val := args[0]
		col2Int, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, errors.New("insertTableOne failed. arg[1] must be convertable to int32")
		}
		col2Val := int32(col2Int)
		col3Int, err := strconv.ParseInt(args[2], 10, 32)
		if err != nil {
			return nil, errors.New("insertTableOne failed. arg[2] must be convertable to int32")
		}
		col3Val := int32(col3Int)

		var columns []*shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
		col3 := shim.Column{Value: &shim.Column_Int32{Int32: col3Val}}
		columns = append(columns, &col1)
		columns = append(columns, &col2)
		columns = append(columns, &col3)

		row := shim.Row{Columns: columns}
		ok, err := stub.InsertRow("tableOne", row)
		if err != nil {
			return nil, fmt.Errorf("insertTableOne operation failed. %s", err)
		}
		if !ok {
			return nil, errors.New("insertTableOne operation failed. Row with given key already exists")
		}

	case "insertRowTableTwo":
		if len(args) < 4 {
			return nil, errors.New("insertRowTableTwo failed. Must include 4 column values")
		}

		col1Val := args[0]
		col2Int, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, errors.New("insertRowTableTwo failed. arg[1] must be convertable to int32")
		}
		col2Val := int32(col2Int)
		col3Int, err := strconv.ParseInt(args[2], 10, 32)
		if err != nil {
			return nil, errors.New("insertRowTableTwo failed. arg[2] must be convertable to int32")
		}
		col3Val := int32(col3Int)
		col4Val := args[3]

		var columns []*shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
		col3 := shim.Column{Value: &shim.Column_Int32{Int32: col3Val}}
		col4 := shim.Column{Value: &shim.Column_String_{String_: col4Val}}
		columns = append(columns, &col1)
		columns = append(columns, &col2)
		columns = append(columns, &col3)
		columns = append(columns, &col4)

		row := shim.Row{Columns: columns}
		ok, err := stub.InsertRow("tableTwo", row)
		if err != nil {
			return nil, fmt.Errorf("insertRowTableTwo operation failed. %s", err)
		}
		if !ok {
			return nil, errors.New("insertRowTableTwo operation failed. Row with given key already exists")
		}

	case "insertRowTableThree":
		if len(args) < 7 {
			return nil, errors.New("insertRowTableThree failed. Must include 7 column values")
		}

		col1Val := args[0]

		col2Int, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, errors.New("insertRowTableThree failed. arg[1] must be convertable to int32")
		}
		col2Val := int32(col2Int)

		col3Val, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return nil, errors.New("insertRowTableThree failed. arg[2] must be convertable to int64")
		}

		col4Uint, err := strconv.ParseUint(args[3], 10, 32)
		if err != nil {
			return nil, errors.New("insertRowTableThree failed. arg[3] must be convertable to uint32")
		}
		col4Val := uint32(col4Uint)

		col5Val, err := strconv.ParseUint(args[4], 10, 64)
		if err != nil {
			return nil, errors.New("insertRowTableThree failed. arg[4] must be convertable to uint64")
		}

		col6Val := []byte(args[5])

		col7Val, err := strconv.ParseBool(args[6])
		if err != nil {
			return nil, errors.New("insertRowTableThree failed. arg[6] must be convertable to bool")
		}

		var columns []*shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
		col3 := shim.Column{Value: &shim.Column_Int64{Int64: col3Val}}
		col4 := shim.Column{Value: &shim.Column_Uint32{Uint32: col4Val}}
		col5 := shim.Column{Value: &shim.Column_Uint64{Uint64: col5Val}}
		col6 := shim.Column{Value: &shim.Column_Bytes{Bytes: col6Val}}
		col7 := shim.Column{Value: &shim.Column_Bool{Bool: col7Val}}
		columns = append(columns, &col1)
		columns = append(columns, &col2)
		columns = append(columns, &col3)
		columns = append(columns, &col4)
		columns = append(columns, &col5)
		columns = append(columns, &col6)
		columns = append(columns, &col7)

		row := shim.Row{Columns: columns}
		ok, err := stub.InsertRow("tableThree", row)
		if err != nil {
			return nil, fmt.Errorf("insertRowTableThree operation failed. %s", err)
		}
		if !ok {
			return nil, errors.New("insertRowTableThree operation failed. Row with given key already exists")
		}

	case "insertRowTableFour":
		if len(args) < 1 {
			return nil, errors.New("insertRowTableFour failed. Must include 1 column value1")
		}

		col1Val := args[0]

		var columns []*shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, &col1)

		row := shim.Row{Columns: columns}
		ok, err := stub.InsertRow("tableFour", row)
		if err != nil {
			return nil, fmt.Errorf("insertRowTableFour operation failed. %s", err)
		}
		if !ok {
			return nil, errors.New("insertRowTableFour operation failed. Row with given key already exists")
		}

	case "deleteRowTableOne":
		if len(args) < 1 {
			return nil, errors.New("deleteRowTableOne failed. Must include 1 key value")
		}

		col1Val := args[0]
		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		err := stub.DeleteRow("tableOne", columns)
		if err != nil {
			return nil, fmt.Errorf("deleteRowTableOne operation failed. %s", err)
		}

	case "replaceRowTableOne":
		if len(args) < 3 {
			return nil, errors.New("replaceRowTableOne failed. Must include 3 column values")
		}

		col1Val := args[0]
		col2Int, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, errors.New("replaceRowTableOne failed. arg[1] must be convertable to int32")
		}
		col2Val := int32(col2Int)
		col3Int, err := strconv.ParseInt(args[2], 10, 32)
		if err != nil {
			return nil, errors.New("replaceRowTableOne failed. arg[2] must be convertable to int32")
		}
		col3Val := int32(col3Int)

		var columns []*shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
		col3 := shim.Column{Value: &shim.Column_Int32{Int32: col3Val}}
		columns = append(columns, &col1)
		columns = append(columns, &col2)
		columns = append(columns, &col3)

		row := shim.Row{Columns: columns}
		ok, err := stub.ReplaceRow("tableOne", row)
		if err != nil {
			return nil, fmt.Errorf("replaceRowTableOne operation failed. %s", err)
		}
		if !ok {
			return nil, errors.New("replaceRowTableOne operation failed. Row with given key does not exist")
		}

	case "deleteAndRecreateTableOne":

		err := stub.DeleteTable("tableOne")
		if err != nil {
			return nil, fmt.Errorf("deleteAndRecreateTableOne operation failed. Error deleting table. %s", err)
		}

		err = createTableOne(stub)
		if err != nil {
			return nil, fmt.Errorf("deleteAndRecreateTableOne operation failed. Error creating table. %s", err)
		}

		return nil, nil

	default:
		return nil, errors.New("Unsupported operation")
	}
	return nil, nil
}

// Query callback representing the query of a chaincode
func (t *SimpleChaincode) Query(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	switch function {

	case "getRowTableOne":
		if len(args) < 1 {
			return nil, errors.New("getRowTableOne failed. Must include 1 key value")
		}

		col1Val := args[0]
		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		row, err := stub.GetRow("tableOne", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowTableOne operation failed. %s", err)
		}

		rowString := fmt.Sprintf("%s", row)
		return []byte(rowString), nil

	case "getRowTableTwo":
		if len(args) < 3 {
			return nil, errors.New("getRowTableTwo failed. Must include 3 key values")
		}

		col1Val := args[0]
		col2Int, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return nil, errors.New("getRowTableTwo failed. arg[1] must be convertable to int32")
		}
		col2Val := int32(col2Int)
		col3Val := args[2]
		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
		col3 := shim.Column{Value: &shim.Column_String_{String_: col3Val}}
		columns = append(columns, col1)
		columns = append(columns, col2)
		columns = append(columns, col3)

		row, err := stub.GetRow("tableTwo", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowTableTwo operation failed. %s", err)
		}

		rowString := fmt.Sprintf("%s", row)
		return []byte(rowString), nil

	case "getRowTableThree":
		if len(args) < 1 {
			return nil, errors.New("getRowTableThree failed. Must include 1 key value")
		}

		col1Val := args[0]

		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		row, err := stub.GetRow("tableThree", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowTableThree operation failed. %s", err)
		}

		rowString := fmt.Sprintf("%s", row)
		return []byte(rowString), nil

	case "getRowsTableTwo":
		if len(args) < 1 {
			return nil, errors.New("getRowsTableTwo failed. Must include at least key values")
		}

		var columns []shim.Column

		col1Val := args[0]
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		if len(args) > 1 {
			col2Int, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil {
				return nil, errors.New("getRowsTableTwo failed. arg[1] must be convertable to int32")
			}
			col2Val := int32(col2Int)
			col2 := shim.Column{Value: &shim.Column_Int32{Int32: col2Val}}
			columns = append(columns, col2)
		}

		rowChannel, err := stub.GetRows("tableTwo", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowsTableTwo operation failed. %s", err)
		}

		var rows []shim.Row
		for {
			select {
			case row, ok := <-rowChannel:
				if !ok {
					rowChannel = nil
				} else {
					rows = append(rows, row)
				}
			}
			if rowChannel == nil {
				break
			}
		}

		jsonRows, err := json.Marshal(rows)
		if err != nil {
			return nil, fmt.Errorf("getRowsTableTwo operation failed. Error marshaling JSON: %s", err)
		}

		return jsonRows, nil

	case "getRowTableFour":
		if len(args) < 1 {
			return nil, errors.New("getRowTableFour failed. Must include 1 key")
		}

		col1Val := args[0]
		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		row, err := stub.GetRow("tableFour", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowTableFour operation failed. %s", err)
		}

		rowString := fmt.Sprintf("%s", row)
		return []byte(rowString), nil

	case "getRowsTableFour":
		if len(args) < 1 {
			return nil, errors.New("getRowsTableFour failed. Must include 1 key value")
		}

		var columns []shim.Column

		col1Val := args[0]
		col1 := shim.Column{Value: &shim.Column_String_{String_: col1Val}}
		columns = append(columns, col1)

		rowChannel, err := stub.GetRows("tableFour", columns)
		if err != nil {
			return nil, fmt.Errorf("getRowsTableFour operation failed. %s", err)
		}

		var rows []shim.Row
		for {
			select {
			case row, ok := <-rowChannel:
				if !ok {
					rowChannel = nil
				} else {
					rows = append(rows, row)
				}
			}
			if rowChannel == nil {
				break
			}
		}

		jsonRows, err := json.Marshal(rows)
		if err != nil {
			return nil, fmt.Errorf("getRowsTableFour operation failed. Error marshaling JSON: %s", err)
		}

		return jsonRows, nil

	default:
		return nil, errors.New("Unsupported operation")
	}
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}

func createTableOne(stub *shim.ChaincodeStub) error {
	// Create table one
	var columnDefsTableOne []*shim.ColumnDefinition
	columnOneTableOneDef := shim.ColumnDefinition{Name: "colOneTableOne",
		Type: shim.ColumnDefinition_STRING, Key: true}
	columnTwoTableOneDef := shim.ColumnDefinition{Name: "colTwoTableOne",
		Type: shim.ColumnDefinition_INT32, Key: false}
	columnThreeTableOneDef := shim.ColumnDefinition{Name: "colThreeTableOne",
		Type: shim.ColumnDefinition_INT32, Key: false}
	columnDefsTableOne = append(columnDefsTableOne, &columnOneTableOneDef)
	columnDefsTableOne = append(columnDefsTableOne, &columnTwoTableOneDef)
	columnDefsTableOne = append(columnDefsTableOne, &columnThreeTableOneDef)
	return stub.CreateTable("tableOne", columnDefsTableOne)
}

func createTableTwo(stub *shim.ChaincodeStub) error {
	var columnDefsTableTwo []*shim.ColumnDefinition
	columnOneTableTwoDef := shim.ColumnDefinition{Name: "colOneTableTwo",
		Type: shim.ColumnDefinition_STRING, Key: true}
	columnTwoTableTwoDef := shim.ColumnDefinition{Name: "colTwoTableTwo",
		Type: shim.ColumnDefinition_INT32, Key: false}
	columnThreeTableTwoDef := shim.ColumnDefinition{Name: "colThreeTableThree",
		Type: shim.ColumnDefinition_INT32, Key: true}
	columnFourTableTwoDef := shim.ColumnDefinition{Name: "colFourTableFour",
		Type: shim.ColumnDefinition_STRING, Key: true}
	columnDefsTableTwo = append(columnDefsTableTwo, &columnOneTableTwoDef)
	columnDefsTableTwo = append(columnDefsTableTwo, &columnTwoTableTwoDef)
	columnDefsTableTwo = append(columnDefsTableTwo, &columnThreeTableTwoDef)
	columnDefsTableTwo = append(columnDefsTableTwo, &columnFourTableTwoDef)
	return stub.CreateTable("tableTwo", columnDefsTableTwo)
}

func createTableThree(stub *shim.ChaincodeStub) error {
	var columnDefsTableThree []*shim.ColumnDefinition
	columnOneTableThreeDef := shim.ColumnDefinition{Name: "colOneTableThree",
		Type: shim.ColumnDefinition_STRING, Key: true}
	columnTwoTableThreeDef := shim.ColumnDefinition{Name: "colTwoTableThree",
		Type: shim.ColumnDefinition_INT32, Key: false}
	columnThreeTableThreeDef := shim.ColumnDefinition{Name: "colThreeTableThree",
		Type: shim.ColumnDefinition_INT64, Key: false}
	columnFourTableThreeDef := shim.ColumnDefinition{Name: "colFourTableFour",
		Type: shim.ColumnDefinition_UINT32, Key: false}
	columnFiveTableThreeDef := shim.ColumnDefinition{Name: "colFourTableFive",
		Type: shim.ColumnDefinition_UINT64, Key: false}
	columnSixTableThreeDef := shim.ColumnDefinition{Name: "colFourTableSix",
		Type: shim.ColumnDefinition_BYTES, Key: false}
	columnSevenTableThreeDef := shim.ColumnDefinition{Name: "colFourTableSeven",
		Type: shim.ColumnDefinition_BOOL, Key: false}
	columnDefsTableThree = append(columnDefsTableThree, &columnOneTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnTwoTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnThreeTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnFourTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnFiveTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnSixTableThreeDef)
	columnDefsTableThree = append(columnDefsTableThree, &columnSevenTableThreeDef)
	return stub.CreateTable("tableThree", columnDefsTableThree)
}

func createTableFour(stub *shim.ChaincodeStub) error {
	var columnDefsTableFour []*shim.ColumnDefinition
	columnOneTableFourDef := shim.ColumnDefinition{Name: "colOneTableFour",
		Type: shim.ColumnDefinition_STRING, Key: true}
	columnDefsTableFour = append(columnDefsTableFour, &columnOneTableFourDef)
	return stub.CreateTable("tableFour", columnDefsTableFour)
}
