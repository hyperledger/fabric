/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

var logger = log.New(os.Stderr, "", 0)

func main() {
	logger.Println("::Detect")

	if err := run(); err != nil {
		logger.Printf("::Error: %v\n", err)
		os.Exit(1)
	}

	logger.Printf("::Type detected as ccaas")

}

type chaincodeMetadata struct {
	Type string `json:"type"`
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("too few arguments")
	}

	chaincodeMetaData := os.Args[2]

	metadataFile := filepath.Join(chaincodeMetaData, "metadata.json")
	_, err := os.Stat(metadataFile)
	if err != nil {
		return fmt.Errorf("%s not found ", metadataFile)
	}

	if _, err := os.Stat(metadataFile); err != nil {
		return errors.WithMessagef(err, "%s not found ", metadataFile)
	}

	mdbytes, cause := ioutil.ReadFile(metadataFile)
	if cause != nil {
		err := errors.WithMessagef(cause, "%s not readable", metadataFile)
		return err
	}

	var metadata chaincodeMetadata
	cause = json.Unmarshal(mdbytes, &metadata)
	if cause != nil {
		return errors.WithMessage(cause, "Unable to parse the metadata.json file")
	}

	if strings.ToLower(metadata.Type) != "ccaas" {
		return fmt.Errorf("chaincode type not supported: %s", metadata.Type)
	}

	// returning nil indicates to the peer a successful detection
	return nil

}
