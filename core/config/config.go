/*
Copyright Greg Haskins <gregory.haskins@gmail.com> 2017, All Rights Reserved.

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

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

func dirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func addConfigPath(v *viper.Viper, p string) {
	if v != nil {
		v.AddConfigPath(p)
	} else {
		viper.AddConfigPath(p)
	}
}

//----------------------------------------------------------------------------------
// GetDevConfigDir()
//----------------------------------------------------------------------------------
// Returns the path to the default configuration that is maintained with the source
// tree.  Only valid to call from a test/development context.
//----------------------------------------------------------------------------------
func GetDevConfigDir() (string, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return "", fmt.Errorf("GOPATH not set")
	}

	for _, p := range filepath.SplitList(gopath) {
		devPath := filepath.Join(p, "src/github.com/hyperledger/fabric/sampleconfig")
		if !dirExists(devPath) {
			continue
		}

		return devPath, nil
	}

	return "", fmt.Errorf("DevConfigDir not found in %s", gopath)
}

//----------------------------------------------------------------------------------
// GetDevMspDir()
//----------------------------------------------------------------------------------
// Builds upon GetDevConfigDir to return the path to our sampleconfig/msp that is
// maintained with the source tree.  Only valid to call from a test/development
// context.  Runtime environment should use configuration elements such as
//
//   GetPath("peer.mspConfigDir")
//----------------------------------------------------------------------------------
func GetDevMspDir() (string, error) {
	devDir, err := GetDevConfigDir()
	if err != nil {
		return "", fmt.Errorf("Error obtaining DevConfigDir: %s", devDir)
	}

	return filepath.Join(devDir, "msp"), nil
}

//----------------------------------------------------------------------------------
// TranslatePath()
//----------------------------------------------------------------------------------
// Translates a relative path into a fully qualified path relative to the config
// file that specified it.  Absolute paths are passed unscathed.
//----------------------------------------------------------------------------------
func TranslatePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}

	return filepath.Join(base, p)
}

//----------------------------------------------------------------------------------
// TranslatePathInPlace()
//----------------------------------------------------------------------------------
// Translates a relative path into a fully qualified path in-place (updating the
// pointer) relative to the config file that specified it.  Absolute paths are
// passed unscathed.
//----------------------------------------------------------------------------------
func TranslatePathInPlace(base string, p *string) {
	*p = TranslatePath(base, *p)
}

//----------------------------------------------------------------------------------
// GetPath()
//----------------------------------------------------------------------------------
// GetPath allows configuration strings that specify a (config-file) relative path
//
// For example: Assume our config is located in /etc/hyperledger/fabric/core.yaml with
// a key "msp.configPath" = "msp/config.yaml".
//
// This function will return:
//      GetPath("msp.configPath") -> /etc/hyperledger/fabric/msp/config.yaml
//
//----------------------------------------------------------------------------------
func GetPath(key string) string {
	p := viper.GetString(key)
	if p == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(viper.ConfigFileUsed()), p)
}

const OfficialPath = "/etc/hyperledger/fabric"

//----------------------------------------------------------------------------------
// InitViper()
//----------------------------------------------------------------------------------
// Performs basic initialization of our viper-based configuration layer.
// Primary thrust is to establish the paths that should be consulted to find
// the configuration we need.  If v == nil, we will initialize the global
// Viper instance
//----------------------------------------------------------------------------------
func InitViper(v *viper.Viper, configName string) error {
	var altPath = os.Getenv("FABRIC_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider
		addConfigPath(v, altPath)
	} else {
		// If we get here, we should use the default paths in priority order:
		//
		// *) CWD
		// *) The $GOPATH based development tree
		// *) /etc/hyperledger/fabric
		//

		// CWD
		addConfigPath(v, "./")

		// DevConfigPath
		err := AddDevConfigPath(v)
		if err != nil {
			return err
		}

		// And finally, the official path
		if dirExists(OfficialPath) {
			addConfigPath(v, OfficialPath)
		}
	}

	// Now set the configuration file.
	if v != nil {
		v.SetConfigName(configName)
	} else {
		viper.SetConfigName(configName)
	}

	return nil
}

//----------------------------------------------------------------------------------
// AddDevConfigPath()
//----------------------------------------------------------------------------------
// Helper utility that automatically adds our DevConfigDir to the viper path
//----------------------------------------------------------------------------------
func AddDevConfigPath(v *viper.Viper) error {
	devPath, err := GetDevConfigDir()
	if err != nil {
		return err
	}

	addConfigPath(v, devPath)

	return nil
}
