/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/otiai10/copy"
	"github.com/pkg/errors"
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

type Config struct {
	PeerName string
}

func main() {
	logger.Println("::Build")

	if err := run(); err != nil {
		logger.Printf("::Error: %v\n", err)
		os.Exit(1)
	}

	logger.Printf("::Build phase completed")

}

func run() error {
	if len(os.Args) < 4 {
		return fmt.Errorf("incorrect number of arguments")
	}

	sourceDir, metadataDir, outputDir := os.Args[1], os.Args[2], os.Args[3]

	connectionSrcFile := filepath.Join(sourceDir, "/connection.json")
	metadataFile := filepath.Clean(filepath.Join(metadataDir, "metadata.json"))
	connectionDestFile := filepath.Join(outputDir, "/connection.json")
	metainfoSrcDir := filepath.Join(sourceDir, "META-INF")
	metainfoDestDir := filepath.Join(outputDir, "META-INF")

	// Process and check the metadata file, then copy to the output location
	if _, err := os.Stat(metadataFile); err != nil {
		return errors.WithMessagef(err, "%s not found ", metadataFile)
	}

	metadataFileContents, cause := ioutil.ReadFile(metadataFile)
	if cause != nil {
		return errors.WithMessagef(cause, "%s file not readable", metadataFile)
	}

	var metadata chaincodeMetadata
	if err := json.Unmarshal(metadataFileContents, &metadata); err != nil {
		return errors.WithMessage(err, "Unable to parse JSON")
	}

	if strings.ToLower(metadata.Type) != "ccaas" {
		return fmt.Errorf("chaincode type should be ccaas, it is %s", metadata.Type)
	}

	if err := copy.Copy(metadataDir, outputDir); err != nil {
		return fmt.Errorf("failed to copy build metadata folder: %s", err)
	}

	if _, err := os.Stat(metainfoSrcDir); !os.IsNotExist(err) {
		if err := copy.Copy(metainfoSrcDir, metainfoDestDir); err != nil {
			return fmt.Errorf("failed to copy build META-INF folder: %s", err)
		}
	}

	// Process and update the connections file
	fileInfo, err := os.Stat(connectionSrcFile)
	if err != nil {
		return errors.WithMessagef(err, "%s not found ", connectionSrcFile)
	}

	connectionFileContents, err := ioutil.ReadFile(connectionSrcFile)
	if err != nil {
		return err
	}

	// read the connection.json file into structure to process
	var connectionData connection
	if err := json.Unmarshal(connectionFileContents, &connectionData); err != nil {
		return err
	}

	// Treat each of the string fields in the connection.json as Go template
	// strings. They can be fixed strings, but if they are templates
	// then the JSON string that is defined in CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG
	// is used as the 'context' to parse the string

	updatedConnection := connection{}
	var cfg map[string]interface{}

	cfgString := os.Getenv("CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG")
	if cfgString != "" {
		if err := json.Unmarshal([]byte(cfgString), &cfg); err != nil {
			return fmt.Errorf("Failed to unmarshal %s", err)
		}
	}

	updatedConnection.Address, err = execTempl(cfg, connectionData.Address)
	if err != nil {
		return fmt.Errorf("Failed to parse the Address field template: %s", err)
	}

	updatedConnection.DialTimeout, err = execTempl(cfg, connectionData.DialTimeout)
	if err != nil {
		return fmt.Errorf("Failed to parse the DialTimeout field template: %s", err)
	}

	// if connection is TLS Enabled, updated with the correct information
	// no other information is needed for the no-TLS case, so the default can be assumed
	// to be good
	if connectionData.TLS {
		updatedConnection.TLS = true
		updatedConnection.ClientAuth = connectionData.ClientAuth

		updatedConnection.RootCert, err = execTempl(cfg, connectionData.RootCert)
		if err != nil {
			return fmt.Errorf("Failed to parse the RootCert field template: %s", err)
		}
		updatedConnection.ClientKey, err = execTempl(cfg, connectionData.ClientKey)
		if err != nil {
			return fmt.Errorf("Failed to parse the ClientKey field template: %s", err)
		}
		updatedConnection.ClientCert, err = execTempl(cfg, connectionData.ClientCert)
		if err != nil {
			return fmt.Errorf("Failed to parse the ClientCert field template: %s", err)
		}
	}

	updatedConnectionBytes, err := json.Marshal(updatedConnection)
	if err != nil {
		return fmt.Errorf("failed to marshal updated connection.json file: %s", err)
	}

	err = ioutil.WriteFile(connectionDestFile, updatedConnectionBytes, fileInfo.Mode())
	if err != nil {
		return err
	}

	return nil

}

// execTempl is a helper function to process a template against a string, and return a string
func execTempl(cfg map[string]interface{}, inputStr string) (string, error) {

	t, err := template.New("").Option("missingkey=error").Parse(inputStr)
	if err != nil {
		fmt.Printf("Failed to parse the template: %s", err)
		return "", err
	}

	buf := &bytes.Buffer{}
	err = t.Execute(buf, cfg)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
