/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/internal/ledger"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	resultFilename      = "./result.json"
	compareErrorMessage = "Ledger Compare Error: "
)

var (
	app = kingpin.New("ledger", "Ledger Utility Tool")

	compare       = app.Command("compare", "Compare two ledgers via their snapshots.")
	snapshotPath1 = compare.Arg("snapshotPath1", "File path to first ledger snapshot.").Required().String()
	snapshotPath2 = compare.Arg("snapshotPath2", "File path to second ledger snapshot.").Required().String()
	all           = compare.Flag("all", "Saves all snapshot divergences to the output file. Default is the earliest 10 divergences.").Short('a').Bool()

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

	switch command {

	case compare.FullCommand():

		count, err := ledger.Compare(*snapshotPath1, *snapshotPath2, resultFilename, *all)
		if err != nil {
			fmt.Printf("%s%s", compareErrorMessage, err)
			return
		}
		fmt.Printf("\nSuccessfully compared snapshots. Result saved to %s. Total differences found: %v\n", resultFilename, count)
		if *all {
			fmt.Println("All found divergences were saved.")
		} else {
			fmt.Println("Only the first 10 found divergences were saved.")
		}

	case troubleshoot.FullCommand():

		fmt.Println("Command TBD")

	}
}
