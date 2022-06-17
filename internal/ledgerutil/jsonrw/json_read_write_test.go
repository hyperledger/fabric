/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jsonrw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/internal/ledgerutil/models"
	"github.com/stretchr/testify/require"
)

const (
	inputJSON = `{
			"ledgerid":"test-channel",
			"diffRecords":[
				{
					"namespace":"_lifecycle$$h_implicit_org_Org1MSP",
					"key":"e01e1c4304282cc5eda5d51c41785bbe49636fbf174514dbd4b98dc9b9ecf5da",
					"hashed":true,
					"snapshotrecord1":null,
					"snapshotrecord2":{
						"value":"0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
						"blockNum":1,
						"txNum":0
					}
				},
				{
					"namespace":"_lifecycle$$h_implicit_org_Org1MSP",
					"key":"e01e1c4304282cc5eda5d51c41795bbe49636fbf174514dbd4b98dc9b9ecf5da",
					"hashed":true,
					"snapshotrecord1":{
						"value":"0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
						"blockNum":8,
						"txNum":0
					},
					"snapshotrecord2":null
				},
				{
					"namespace":"marbles",
					"key":"marble1",
					"hashed":false,
					"snapshotrecord1":{
						"value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\"}",
						"blockNum":3,
						"txNum":0
					},
					"snapshotrecord2":{
						"value":"{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"jerry\"}",
						"blockNum":4,
						"txNum":0
					}
				}
			]
		}`
	expectedOutputJSON = "{\n" +
		"\"field1\":\"value1\"\n" +
		",\n" +
		"\"field2\":[\n" +
		"{\"label\":\"abc\",\"num\":7}\n" +
		",\n" +
		"{\"label\":\"xyz\",\"num\":99}\n" +
		"]\n" +
		",\n" +
		"\"field3\":\"value3\"\n" +
		"}\n"
)

type sampleObject struct {
	Label string `json:"label"`
	Num   uint64 `json:"num"`
}

func TestLoadRecords(t *testing.T) {
	fp, err := createTestJSONInput(t, inputJSON)
	require.NoError(t, err)

	expectedDiffRec1 := models.DiffRecord{
		Namespace: "_lifecycle$$h_implicit_org_Org1MSP",
		Key:       "e01e1c4304282cc5eda5d51c41785bbe49636fbf174514dbd4b98dc9b9ecf5da",
		Hashed:    true,
		Record1:   nil,
		Record2: &models.SnapshotRecord{
			Value:    "0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
			BlockNum: uint64(1),
			TxNum:    uint64(0),
		},
	}
	expectedDiffRec2 := models.DiffRecord{
		Namespace: "_lifecycle$$h_implicit_org_Org1MSP",
		Key:       "e01e1c4304282cc5eda5d51c41795bbe49636fbf174514dbd4b98dc9b9ecf5da",
		Hashed:    true,
		Record1: &models.SnapshotRecord{
			Value:    "0c52a3dbc7b322ff35728afdd691244cfc0fc9c4743c254b57059a2394e14daf",
			BlockNum: uint64(8),
			TxNum:    uint64(0),
		},
		Record2: nil,
	}
	expectedDiffRec3 := models.DiffRecord{
		Namespace: "marbles",
		Key:       "marble1",
		Hashed:    false,
		Record1: &models.SnapshotRecord{
			Value:    "{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"tom\"}",
			BlockNum: uint64(3),
			TxNum:    uint64(0),
		},
		Record2: &models.SnapshotRecord{
			Value:    "{\"docType\":\"marble\",\"name\":\"marble1\",\"color\":\"blue\",\"size\":35,\"owner\":\"jerry\"}",
			BlockNum: uint64(4),
			TxNum:    uint64(0),
		},
	}
	expectedDiffRecList := models.DiffRecordList{
		Ledgerid: "test-channel",
		DiffRecords: []*models.DiffRecord{
			&expectedDiffRec1, &expectedDiffRec2, &expectedDiffRec3,
		},
	}

	diffRecList, err := LoadRecords(fp)
	require.NoError(t, err)
	require.Equal(t, &expectedDiffRecList, diffRecList)
}

// Turns input string into json file, returns filepath
func createTestJSONInput(t *testing.T, input string) (string, error) {
	// Temp directory and file for input json
	inputDir := t.TempDir()
	fp := filepath.Join(inputDir, "testInput.json")
	f, err := os.Create(fp)
	if err != nil {
		return "", err
	}
	// Write to json file
	f.WriteString(input)
	err = f.Close()
	if err != nil {
		return "", err
	}
	return fp, nil
}

func TestJSONFileWriter(t *testing.T) {
	// Temp directory and file for output json
	outputDir := t.TempDir()
	fp := filepath.Join(outputDir, "testOutput.json")
	// New JSONFileWriter
	jsonFileWriter, err := NewJSONFileWriter(fp)
	require.NoError(t, err)
	err = jsonFileWriter.OpenObject()
	require.NoError(t, err)
	// New field
	err = jsonFileWriter.AddField("field1", "value1")
	require.NoError(t, err)
	var emptySlice []interface{}
	// New list
	err = jsonFileWriter.AddField("field2", emptySlice)
	require.NoError(t, err)
	// Add entries
	sampleObj1 := sampleObject{
		Label: "abc",
		Num:   uint64(7),
	}
	sampleObj2 := sampleObject{
		Label: "xyz",
		Num:   uint64(99),
	}
	err = jsonFileWriter.AddEntry(sampleObj1)
	require.NoError(t, err)
	err = jsonFileWriter.AddEntry(sampleObj2)
	require.NoError(t, err)
	err = jsonFileWriter.CloseList()
	require.NoError(t, err)
	// Add field
	err = jsonFileWriter.AddField("field3", "value3")
	require.NoError(t, err)
	// Close
	err = jsonFileWriter.CloseObject()
	require.NoError(t, err)
	err = jsonFileWriter.Close()
	require.NoError(t, err)
	// Read from generated output file
	output, err := os.ReadFile(fp)
	require.NoError(t, err)
	require.Equal(t, expectedOutputJSON, string(output))
}
