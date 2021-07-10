/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/internal/ledgerutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	resultFilename = "result.json"
)

var (
	app = kingpin.New("ledgerutil", "Ledger Utility Tool")

	compare       = app.Command("compare", "Compare two ledgers via their snapshots.")
	snapshotPath1 = compare.Arg("snapshotPath1", "First ledger snapshot directory.").Required().String()
	snapshotPath2 = compare.Arg("snapshotPath2", "Second ledger snapshot directory.").Required().String()
	outputDir     = compare.Flag("outputDir", "Snapshot comparison json results output directory. Default is the current directory.").Short('o').String()

	troubleshoot = app.Command("troubleshoot", "Identify potentially divergent transactions.")

	args = os.Args[1:]
)

func main() {
	kingpin.Version("0.0.1")

	command, err := app.Parse(args)
	if err != nil {
		kingpin.Fatalf("parsing arguments: %s. Try --help", err)
		return
	}

	// Determine result json file location
	var resultFilepath string
	if outputDir != nil {
		resultFilepath = *outputDir
	} else {
		resultFilepath, err = os.Getwd()
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	resultFilepath = filepath.Join(resultFilepath, resultFilename)

	// Command logic
	switch command {

	case compare.FullCommand():

		count, err := ledgerutil.Compare(*snapshotPath1, *snapshotPath2, resultFilepath)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("\nSuccessfully compared snapshots. Result saved to %s. Total differences found: %d\n", resultFilepath, count)

	case troubleshoot.FullCommand():

		fmt.Println("Command TBD")

	}
}
