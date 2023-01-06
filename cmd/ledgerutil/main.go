/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/internal/ledgerutil/compare"
	"github.com/hyperledger/fabric/internal/ledgerutil/identifytxs"
	"github.com/hyperledger/fabric/internal/ledgerutil/verify"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	compareErrorMessage = "Ledger Compare Error: "
	outputDirDesc       = "Snapshot comparison json results output directory. Default is the current directory."
	firstDiffsDesc      = "Maximum number of differences to record in " + compare.FirstDiffsByHeight +
		". Requesting a report with many differences may result in a large amount of memory usage. Defaults " +
		" to 10. If set to 0, will not produce " + compare.FirstDiffsByHeight + "."
	identifytxsErrorMessage = "Ledger Identify Transactions Error: "
	snapshotDiffsPathDesc   = "Path to json file containing list of target records to search for in transactions. This is typically output " +
		"from ledgerutil compare."
	blockStorePathDesc = "Path to file system of target peer, used to access block store. Defaults to '/var/hyperledger/production'. " +
		"IMPORTANT: If the configuration for target peer's file system path was changed, the new path MUST be provided."
	blockStorePathDefault = "/var/hyperledger/production"
	outputDirIdDesc       = "Location for identified transactions json results output directory. Default is the current directory."
	verifyErrorMessage    = "Verify Ledger Error:"
	outputDirVerifyDesc   = "Location for verification result output directory. Default is the current directory."
)

var (
	app = kingpin.New("ledgerutil", "Ledger Utility Tool")

	compareApp    = app.Command("compare", "Compare channel snapshots from two different peers.")
	snapshotPath1 = compareApp.Arg("snapshotPath1", "First ledger snapshot directory.").Required().String()
	snapshotPath2 = compareApp.Arg("snapshotPath2", "Second ledger snapshot directory.").Required().String()
	outputDir     = compareApp.Flag("outputDir", outputDirDesc).Short('o').String()
	firstDiffs    = compareApp.Flag("firstDiffs", firstDiffsDesc).Short('f').Default("10").Int()

	identifytxsApp    = app.Command("identifytxs", "Identify potentially divergent transactions.")
	snapshotDiffsPath = identifytxsApp.Arg("snapshotDiffsPath", snapshotDiffsPathDesc).Required().String()
	blockStorePath    = identifytxsApp.Arg("blockStorePath", blockStorePathDesc).Default(blockStorePathDefault).String()
	outputDirId       = identifytxsApp.Flag("outputDir", outputDirIdDesc).Short('o').String()

	verifyApp            = app.Command("verify", "Verify the integrity of a ledger")
	blockStorePathVerify = verifyApp.Arg("blockStorePath", blockStorePathDesc).Default(blockStorePathDefault).String()
	outputDirVerify      = verifyApp.Flag("outputDir", outputDirVerifyDesc).Short('o').String()

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
	case compareApp.FullCommand():

		// Determine result json file location
		if *outputDir == "" {
			*outputDir, err = os.Getwd()
			if err != nil {
				fmt.Printf("%s%s\n", compareErrorMessage, err)
				os.Exit(1)
			}
		}

		count, outputDirPath, err := compare.Compare(*snapshotPath1, *snapshotPath2, *outputDir, *firstDiffs)
		if err != nil {
			fmt.Printf("%s%s\n", compareErrorMessage, err)
			os.Exit(1)
		}

		fmt.Print("\nSuccessfully compared snapshots. ")
		if count == -1 {
			fmt.Println("Both snapshot public state and private state hashes were the same. No results were generated.")
		} else {
			fmt.Printf("Results saved to %s. Total differences found: %d\n", outputDirPath, count)
			os.Exit(2)
		}

	case identifytxsApp.FullCommand():

		// Determine result json file location
		if *outputDirId == "" {
			*outputDirId, err = os.Getwd()
			if err != nil {
				fmt.Printf("%s%s\n", identifytxsErrorMessage, err)
				os.Exit(1)
			}
		}

		firstBlock, lastBlock, err := identifytxs.IdentifyTxs(*snapshotDiffsPath, *blockStorePath, *outputDirId)
		if err != nil {
			fmt.Printf("%s%s\n", identifytxsErrorMessage, err)
			os.Exit(1)
		}
		if firstBlock > lastBlock {
			fmt.Printf("%sFirst available block was greater than highest record. No transactions were checked.", identifytxsErrorMessage)
			os.Exit(1)
		}
		fmt.Printf("\nSuccessfully ran identify transactions tool. Transactions were checked between blocks %d and %d.", firstBlock, lastBlock)

	case verifyApp.FullCommand():

		// Determine result json file location
		if *outputDirVerify == "" {
			*outputDirVerify, err = os.Getwd()
			if err != nil {
				fmt.Printf("%s%s\n", verifyErrorMessage, err)
				os.Exit(1)
			}
		}

		valid, err := verify.VerifyLedger(*blockStorePathVerify, *outputDirVerify)
		if err != nil {
			fmt.Printf("%s%s\n", verifyErrorMessage, err)
			os.Exit(1)
		}

		if valid {
			fmt.Printf("\nSuccessfully executed verify tool. No error is found.\n")
		} else {
			fmt.Printf("\nSuccessfully executed verify tool. Some error(s) are found.\n")
			os.Exit(1)
		}
	}
}
