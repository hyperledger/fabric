/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jsonrw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/internal/ledgerutil/models"
	"github.com/pkg/errors"
)

// json Reading

// Loads a json file containing a DiffRecordList object
func LoadRecords(filePath string) (*models.DiffRecordList, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	var records *models.DiffRecordList

	d := json.NewDecoder(bufio.NewReader(f))
	err = d.Decode(&records)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return records, nil
}

// json Writing

// JSONFileWriter writes data to a json file
type JSONFileWriter struct {
	file              *os.File
	buffer            *bufio.Writer
	encoder           *json.Encoder
	objectOpened      bool
	firstFieldWritten bool
	listOpened        bool
	firstEntryWritten bool
	count             int
}

func NewJSONFileWriter(filePath string) (*JSONFileWriter, error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	b := bufio.NewWriter(f)

	return &JSONFileWriter{
		file:    f,
		buffer:  b,
		encoder: json.NewEncoder(b),
	}, nil
}

// Open a json object
func (w *JSONFileWriter) OpenObject() error {
	if w.objectOpened {
		return errors.Errorf("object already open, must close object before starting a new one")
	}

	w.objectOpened = true
	_, err := w.buffer.Write([]byte("{\n"))
	if err != nil {
		return err
	}

	return nil
}

// Close a json object
func (w *JSONFileWriter) CloseObject() error {
	if !w.objectOpened {
		return errors.Errorf("no object open, cannot close object")
	}

	_, err := w.buffer.Write([]byte("}\n"))
	if err != nil {
		return err
	}

	w.objectOpened = false
	w.firstFieldWritten = false

	return nil
}

// Add field to an open json object
func (w *JSONFileWriter) AddField(k string, v interface{}) error {
	// Need to open object before adding fields
	if !w.objectOpened {
		return errors.Errorf("no object open, cannot add field")
	}
	// Add commas for fields after the first field in the object
	if w.firstFieldWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	} else {
		w.firstFieldWritten = true
	}
	// Key : Value
	keyString := fmt.Sprintf("\"%s\":", k)
	_, err := w.buffer.Write([]byte(keyString))
	if err != nil {
		return err
	}
	// Determine if v is list
	if reflect.TypeOf(v).Kind() == reflect.Slice {
		err = w.OpenList()
		if err != nil {
			return err
		}
	} else {
		err = w.encoder.Encode(v)
		if err != nil {
			return err
		}
	}

	return nil
}

// Open a json list
func (w *JSONFileWriter) OpenList() error {
	if w.listOpened {
		return errors.Errorf("list already open, must close list before starting a new one")
	}

	w.listOpened = true
	w.count = 0
	_, err := w.buffer.Write([]byte("[\n"))
	if err != nil {
		return err
	}

	return nil
}

// Close a json list
func (w *JSONFileWriter) CloseList() error {
	if !w.listOpened {
		return errors.Errorf("no list open, cannot close list")
	}

	w.listOpened = false
	_, err := w.buffer.Write([]byte("]\n"))
	if err != nil {
		return err
	}

	return nil
}

// Add entries to an open json list
func (w *JSONFileWriter) AddEntry(r interface{}) error {
	// Need to open list before adding entries
	if !w.listOpened {
		return errors.Errorf("no list open, cannot add entries")
	}
	// Add commas for entries after the first entry in the list
	if w.firstEntryWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	} else {
		w.firstEntryWritten = true
	}

	err := w.encoder.Encode(r)
	if err != nil {
		return err
	}
	w.count++

	return nil
}

func (w *JSONFileWriter) Close() error {
	if w.listOpened {
		return errors.Errorf("list still open, must close list before closing jsonFileWriter")
	}

	err := w.buffer.Flush()
	if err != nil {
		return err
	}

	err = w.file.Sync()
	if err != nil {
		return err
	}

	err = fileutil.SyncParentDir(w.file.Name())
	if err != nil {
		return err
	}

	return w.file.Close()
}
