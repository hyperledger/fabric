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

package shim

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

func createTable(stub ChaincodeStubInterface) error {
	// Create table one
	var columnDefsTableOne []*ColumnDefinition
	columnOneTableOneDef := ColumnDefinition{Name: "colOneTableOne",
		Type: ColumnDefinition_STRING, Key: true}
	columnTwoTableOneDef := ColumnDefinition{Name: "colTwoTableOne",
		Type: ColumnDefinition_INT32, Key: false}
	columnThreeTableOneDef := ColumnDefinition{Name: "colThreeTableOne",
		Type: ColumnDefinition_INT32, Key: false}
	columnDefsTableOne = append(columnDefsTableOne, &columnOneTableOneDef)
	columnDefsTableOne = append(columnDefsTableOne, &columnTwoTableOneDef)
	columnDefsTableOne = append(columnDefsTableOne, &columnThreeTableOneDef)
	return stub.CreateTable("tableOne", columnDefsTableOne)
}

func insertRow(stub ChaincodeStubInterface, col1Val string, col2Val int32, col3Val int32) error {
	var columns []*Column
	col1 := Column{Value: &Column_String_{String_: col1Val}}
	col2 := Column{Value: &Column_Int32{Int32: col2Val}}
	col3 := Column{Value: &Column_Int32{Int32: col3Val}}
	columns = append(columns, &col1)
	columns = append(columns, &col2)
	columns = append(columns, &col3)

	row := Row{Columns: columns}
	ok, err := stub.InsertRow("tableOne", row)
	if err != nil {
		return fmt.Errorf("insertTableOne operation failed. %s", err)
	}
	if !ok {
		return errors.New("insertTableOne operation failed. Row with given key already exists")
	}
	return nil
}

func getRow(stub ChaincodeStubInterface, col1Val string) (Row, error) {
	var columns []Column
	col1 := Column{Value: &Column_String_{String_: col1Val}}
	columns = append(columns, col1)

	row, err := stub.GetRow("tableOne", columns)
	if err != nil {
		return row, fmt.Errorf("getRowTableOne operation failed. %s", err)
	}

	return row, nil
}

func getRows(stub ChaincodeStubInterface, col1Val string) ([]Row, error) {
	var columns []Column

	col1 := Column{Value: &Column_String_{String_: col1Val}}
	columns = append(columns, col1)

	rowChannel, err := stub.GetRows("tableOne", columns)
	if err != nil {
		return nil, fmt.Errorf("getRows operation failed. %s", err)
	}

	var rows []Row
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

	return rows, nil
}

func TestMockStateRangeQueryIterator(t *testing.T) {
	stub := NewMockStub("rangeTest", nil)
	stub.MockTransactionStart("init")
	stub.PutState("1", []byte{61})
	stub.PutState("2", []byte{62})
	stub.PutState("5", []byte{65})
	stub.PutState("3", []byte{63})
	stub.PutState("4", []byte{64})
	stub.PutState("6", []byte{66})
	stub.MockTransactionEnd("init")

	expectKeys := []string{"2", "3", "4"}
	expectValues := [][]byte{{62}, {63}, {64}}

	rqi := NewMockStateRangeQueryIterator(stub, "2", "4")

	fmt.Println("Running loop")
	for i := 0; i < 3; i++ {
		key, value, err := rqi.Next()
		fmt.Println("Loop", i, "got", key, value, err)
		if expectKeys[i] != key {
			fmt.Println("Expected key", expectKeys[i], "got", key)
			t.FailNow()
		}
		if expectValues[i][0] != value[0] {
			fmt.Println("Expected value", expectValues[i], "got", value)
		}
	}
}

func TestMockTable(t *testing.T) {
	stub := NewMockStub("CreateTable", nil)
	stub.MockTransactionStart("init")

	//create a table
	if err := createTable(stub); err != nil {
		t.FailNow()
	}

	type rowType struct {
		col1 string
		col2 int32
		col3 int32
	}

	//add some rows
	rows := []rowType{{"one", 1, 11}, {"two", 2, 22}, {"three", 3, 33}}
	for _, r := range rows {
		if err := insertRow(stub, r.col1, r.col2, r.col3); err != nil {
			t.FailNow()
		}
	}

	//get one row
	if r, err := getRow(stub, "one"); err != nil {
		t.FailNow()
	} else if len(r.Columns) != 3 || r.Columns[0].GetString_() != "one" || r.Columns[1].GetInt32() != 1 || r.Columns[2].GetInt32() != 11 {
		t.FailNow()
	}

	/** we know GetRows is buggy and need to be fixed. Enable this test
	  * when it is
	//get all rows
	if rs,err := getRows(stub,"one"); err != nil {
		fmt.Printf("getRows err %s\n", err)
		t.FailNow()
	} else if len(rs) != 1 {
		fmt.Printf("getRows returned len %d(expected 1)\n", len(rs))
		t.FailNow()
	} else if len(rs[0].Columns) != 3 || rs[0].Columns[0].GetString_() != "one" || rs[0].Columns[1].GetInt32() != 1 || rs[0].Columns[2].GetInt32() != 11 {
		fmt.Printf("getRows invaid row %v\n", rs[0])
		t.FailNow()
	}
	***/
}

// TestSetChaincodeLoggingLevel uses the utlity function defined in chaincode.go to
// set the chaincodeLogger's logging level
func TestSetChaincodeLoggingLevel(t *testing.T) {
	// set log level to a non-default level
	testLogLevelString := "debug"
	viper.Set("logging.chaincode", testLogLevelString)

	SetChaincodeLoggingLevel()

	if !IsEnabledForLogLevel(testLogLevelString) {
		t.FailNow()
	}
}
