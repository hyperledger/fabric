/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"fmt"
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
}

type chaincodeMetadata struct {
	Type string `json:"type"`
}

func run() error {
	if len(os.Args) < 3 {
		return errors.New("too few arguments")
	}

	chaincodeMetaData := os.Args[2]

	// Check metadata file's existence
	metadataFile := filepath.Join(chaincodeMetaData, "metadata.json")
	if _, err := os.Stat(metadataFile); err != nil {
		return errors.WithMessagef(err, "%s not found ", metadataFile)
	}

	// Read the metadata file
	mdbytes, err := os.ReadFile(metadataFile)
	if err != nil {
		return errors.WithMessagef(err, "%s not readable", metadataFile)
	}

	var metadata chaincodeMetadata
	err = json.Unmarshal(mdbytes, &metadata)
	if err != nil {
		return errors.WithMessage(err, "Unable to parse the metadata.json file")
	}

	if strings.ToLower(metadata.Type) != "ccaas" && strings.ToLower(metadata.Type) != "remote" {
		return fmt.Errorf("chaincode type not supported: %s", metadata.Type)
	}

	logger.Printf("::Type detected as: " + strings.ToLower(metadata.Type))

	// returning nil indicates to the peer a successful detection
	return nil
}
