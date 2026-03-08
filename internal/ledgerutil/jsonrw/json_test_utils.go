/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jsonrw

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
)

func OutputFileToString(f string, path string) (string, error) {
	fpath := filepath.Join(path, f)
	exists, err := outputFileExists(fpath)
	if err != nil {
		return "", err
	}
	if exists {
		b, err := os.ReadFile(fpath)
		if err != nil {
			return "", err
		}
		out, err := io.ReadAll(bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	return "null", nil
}

func outputFileExists(f string) (bool, error) {
	_, err := os.Stat(f)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
