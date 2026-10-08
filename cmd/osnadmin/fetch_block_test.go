/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"strings"
	"testing"
)

func TestFetchRejectsNonCanonicalBlockID(t *testing.T) {
	for _, blockID := range []string{"-1", "+1"} {
		t.Run(blockID, func(t *testing.T) {
			output, exitCode, err := executeForArgs([]string{
				"--orderer-address", "127.0.0.1:7053",
				"channel", "fetch",
				"--channelID", "mychannel",
				"--blockID=" + blockID,
				"--outputfile", "block.pb",
			})
			if err == nil {
				t.Fatalf("blockID %s: got output %q exit %d, want an error", blockID, output, exitCode)
			}
			if exitCode != 1 {
				t.Fatalf("exit %d, want 1: %v", exitCode, err)
			}
			if !strings.Contains(err.Error(), "not equal") {
				t.Fatalf("error %q", err)
			}
		})
	}
}
