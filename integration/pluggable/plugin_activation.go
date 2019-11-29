/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pluggable

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	EndorsementPluginEnvVar = "ENDORSEMENT_PLUGIN_ENV_VAR"
	ValidationPluginEnvVar  = "VALIDATION_PLUGIN_ENV_VAR"
)

// EndorsementPluginActivationFolder returns the name of the folder that if
// the file of the peer's id in it exists - it indicates that the endorsement plugin was activated
// for that peer
func EndorsementPluginActivationFolder() string {
	return os.Getenv(EndorsementPluginEnvVar)
}

// SetEndorsementPluginActivationFolder sets the name of the folder
// that if the file of the peer's id in it exists - it indicates that the endorsement plugin was activated
// for that peer
func SetEndorsementPluginActivationFolder(path string) {
	os.Setenv(EndorsementPluginEnvVar, path)
}

// ValidationPluginActivationFilePath returns the name of the folder that if
// the file of the peer's id in it exists - it indicates that the validation plugin was activated
// for that peer
func ValidationPluginActivationFolder() string {
	return os.Getenv(ValidationPluginEnvVar)
}

// SetValidationPluginActivationFolder sets the name of the folder
// that if the file of the peer's id in it exists - it indicates that the validation plugin was activated
// for that peer
func SetValidationPluginActivationFolder(path string) {
	os.Setenv(ValidationPluginEnvVar, path)
}

func markPluginActivation(dir string) {
	fileName := filepath.Join(dir, viper.GetString("peer.id"))
	_, err := os.Create(fileName)
	if err != nil {
		panic(fmt.Sprintf("failed to create file %s: %v", fileName, err))
	}
}

// PublishEndorsementPluginActivation makes it known that the endorsement plugin
// was activated for the peer that is invoking this function
func PublishEndorsementPluginActivation() {
	markPluginActivation(EndorsementPluginActivationFolder())
}

// PublishValidationPluginActivation makes it known that the validation plugin
// was activated for the peer that is invoking this function
func PublishValidationPluginActivation() {
	markPluginActivation(ValidationPluginActivationFolder())
}

// CountEndorsementPluginActivations returns the number of peers that activated
// the endorsement plugin
func CountEndorsementPluginActivations() int {
	return listDir(EndorsementPluginActivationFolder())
}

// CountValidationPluginActivations returns the number of peers that activated
// the validation plugin
func CountValidationPluginActivations() int {
	return listDir(ValidationPluginActivationFolder())
}

func listDir(d string) int {
	dir, err := ioutil.ReadDir(d)
	if err != nil {
		panic(fmt.Sprintf("failed listing directory %s: %v", d, err))
	}
	return len(dir)
}
