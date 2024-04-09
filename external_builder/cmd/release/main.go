/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/otiai10/copy"
)

var logger = log.New(os.Stderr, "", 0)

type chaincodeMetadata struct {
	Type string `json:"type"`
}

// Connection structure is used to represent the
// connection.json file that is supplied in the
// chaincode package.
type connection struct {
	Address     string `json:"address"`
	DialTimeout string `json:"dial_timeout"`
	TLS         bool   `json:"tls_required"`
	ClientAuth  bool   `json:"client_auth_required"`
	RootCert    string `json:"root_cert"`
	ClientKey   string `json:"client_key"`
	ClientCert  string `json:"client_cert"`
}

func main() {
	logger.Println("::Release")

	if err := run(); err != nil {
		logger.Printf("::Error: %v\n", err)
		os.Exit(1)
	}
	logger.Printf("::Release phase completed")

}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("incorrect number of arguments")
	}

	builderOutputDir, releaseDir := os.Args[1], os.Args[2]
	connectionSrcFile := filepath.Join(builderOutputDir, "/connection.json")

	connectionDir := filepath.Join(releaseDir, "chaincode/server/")
	connectionDestFile := filepath.Join(releaseDir, "chaincode/server/connection.json")

	metadataDir := filepath.Join(builderOutputDir, "META-INF/statedb")
	metadataDestDir := filepath.Join(releaseDir, "statedb")
	if _, err := os.Stat(metadataDir); !os.IsNotExist(err) {
		if err := copy.Copy(metadataDir, metadataDestDir); err != nil {
			return fmt.Errorf("failed to copy metadataDir directory folder: %s", err)
		}
	}

	// Process and update the connections file
	_, err := os.Stat(connectionSrcFile)
	if err != nil {
		return fmt.Errorf("connection.json not found in source folder: %s", err)
	}

	err = os.MkdirAll(connectionDir, 0750)
	if err != nil {
		return fmt.Errorf("failed to create target folder for connection.json: %s", err)
	}

	if err = Copy(connectionSrcFile, connectionDestFile); err != nil {
		return err
	}

	return nil

}

// Copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
func Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}
