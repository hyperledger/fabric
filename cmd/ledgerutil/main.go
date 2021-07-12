/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/internal/ledgerutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	compareErrorMessage = "Ledger Compare Error: "
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

	// Set results directory location; an empty location sets the current directory by default
	if *outputDir != "" {
		err = os.Chdir(*outputDir)
		if err != nil {
			fmt.Printf("%s%s\n", compareErrorMessage, err)
			return
		}
	}
	*outputDir, err = os.Getwd()
	if err != nil {
		fmt.Printf("%s%s\n", compareErrorMessage, err)
		return
	}

	// Command logic
	switch command {

	case compare.FullCommand():

		count, _, err := ledgerutil.Compare(*snapshotPath1, *snapshotPath2, *outputDir)
		if err != nil {
			fmt.Printf("%s%s\n", compareErrorMessage, err)
			return
		}
		fmt.Printf("\nSuccessfully compared snapshots. Result saved to %s. Total differences found: %d\n", *outputDir, count)

	case troubleshoot.FullCommand():

		fmt.Println("Command TBD")

	}
}
