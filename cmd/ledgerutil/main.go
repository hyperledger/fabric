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
	outputDirDesc       = "Snapshot comparison json results output directory. Default is the current directory."
	firstDiffsDesc      = "Maximum number of differences to record in " + ledgerutil.FirstDiffsByHeight +
		". Requesting a report with many differences may result in a large amount " +
		"of memory usage. Defaults to 10. If set to 0, will not produce " +
		ledgerutil.FirstDiffsByHeight + "."
)

var (
	app = kingpin.New("ledgerutil", "Ledger Utility Tool")

	compare       = app.Command("compare", "Compare channel snapshots from two different peers.")
	snapshotPath1 = compare.Arg("snapshotPath1", "First ledger snapshot directory.").Required().String()
	snapshotPath2 = compare.Arg("snapshotPath2", "Second ledger snapshot directory.").Required().String()
	outputDir     = compare.Flag("outputDir", outputDirDesc).Short('o').String()
	firstDiffs    = compare.Flag("firstDiffs", firstDiffsDesc).Short('f').Default("10").Int()

	args = os.Args[1:]
)

func main() {
	kingpin.Version("0.0.1")

	command, err := app.Parse(args)
	if err != nil {
		kingpin.Fatalf("parsing arguments: %s. Try --help", err)
		return
	}

	// Command logic
	switch command {
	case compare.FullCommand():

		// Determine result json file location
		if *outputDir == "" {
			*outputDir, err = os.Getwd()
			if err != nil {
				fmt.Printf("%s%s\n", compareErrorMessage, err)
				return
			}
		}

		count, outputDirPath, err := ledgerutil.Compare(*snapshotPath1, *snapshotPath2, *outputDir, *firstDiffs)
		if err != nil {
			fmt.Printf("%s%s\n", compareErrorMessage, err)
			return
		}

		fmt.Print("\nSuccessfully compared snapshots. ")
		if count == -1 {
			fmt.Println("Both snapshot public state and private state hashes were the same. No results were generated.")
		} else {
			fmt.Printf("Results saved to %s. Total differences found: %d\n", outputDirPath, count)
		}
	}
}
