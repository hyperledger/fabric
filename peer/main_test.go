/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildcommandNameOutputSingleCommand(t *testing.T) {
	command := &cobra.Command{Use: "command"}

	commandNameOutput := getPeerCommandFromCobraCommand(command)

	assertEqual(t, "command", commandNameOutput)
}

func TestBuildcommandNameOutputNilCommand(t *testing.T) {
	var command *cobra.Command

	commandNameOutput := getPeerCommandFromCobraCommand(command)

	assertEqual(t, "", commandNameOutput)
}

func TestBuildcommandNameOutputTwoCommands(t *testing.T) {
	rootCommand := &cobra.Command{Use: "rootcommand"}
	childCommand := &cobra.Command{Use: "childcommand"}
	rootCommand.AddCommand(childCommand)

	commandNameOutput := getPeerCommandFromCobraCommand(childCommand)

	assertEqual(t, "childcommand", commandNameOutput)
}

func TestBuildcommandNameOutputThreeCommands(t *testing.T) {
	rootCommand := &cobra.Command{Use: "rootcommand"}
	childCommand := &cobra.Command{Use: "childcommand"}
	leafCommand := &cobra.Command{Use: "leafCommand"}
	rootCommand.AddCommand(childCommand)
	childCommand.AddCommand(leafCommand)

	commandNameOutput := getPeerCommandFromCobraCommand(leafCommand)

	assertEqual(t, "childcommand", commandNameOutput)
}

func TestBuildcommandNameOutputFourCommands(t *testing.T) {
	rootCommand := &cobra.Command{Use: "rootcommand"}
	childCommand := &cobra.Command{Use: "childcommand"}
	secondChildCommand := &cobra.Command{Use: "secondChildCommand"}
	leafCommand := &cobra.Command{Use: "leafCommand"}

	rootCommand.AddCommand(childCommand)
	childCommand.AddCommand(secondChildCommand)
	secondChildCommand.AddCommand(leafCommand)

	commandNameOutput := getPeerCommandFromCobraCommand(leafCommand)

	assertEqual(t, "childcommand", commandNameOutput)
}

func assertEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}
