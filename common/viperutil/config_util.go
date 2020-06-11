/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package viperutil

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	version "github.com/hashicorp/go-version"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var logger = flogging.MustGetLogger("viperutil")

// prototype for get environment variable
type envGetter func(key string) interface{}

// default config file path
const OfficialPath = "/etc/hyperledger/fabric"

var SupportedExts []string = []string{"yaml", "yml"}

// ConfigParser holds the configuration file locations.
// It keeps the config file directory locations and env. variables
// From the file the config is unmarshalled and stored.
// Currently "yaml" is supported.
type ConfigParser struct {

	// configuration file to process
	configPaths []string
	configName  string
	configFile  string
	configType  string

	// env variable prefix
	envPrefix           string
	automaticEnvApplied bool
	envKeyReplacer      *strings.Replacer

	// parsed config
	config map[string]interface{}
}

// Creates a ConfigParser instance
func New() *ConfigParser {
	c := new(ConfigParser)
	c.config = make(map[string]interface{})
	return c
}

// Keeps a list of path to search the relevant
// config file. Multiple paths can be provided.
func (c *ConfigParser) AddConfigPath(in string) {
	if in != "" {
		logger.Debugf("Adding search path : %s", in)
		for _, b := range c.configPaths {
			if b == in {
				return
			}
		}
		c.configPaths = append(c.configPaths, in)
	}
}

// The config file name. The extension is not included
func (c *ConfigParser) SetConfigName(in string) {
	if in != "" {
		c.configName = in
	}
}

// The extension of the config file
// Currently only "yaml" is supported.
func (c *ConfigParser) SetConfigType(in string) {
	if in != "" {
		c.configType = in
	}
}

// Method to initialize default paths.
// The defaults paths are CWD and OfficialPath
// The env variable FABRIC_CFG_PATH can be used to override default
func (c *ConfigParser) InitConfigPath(configName string) error {
	var altPath = os.Getenv("FABRIC_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider

		fi, err := os.Stat(altPath)
		if err != nil || !fi.IsDir() {
			return fmt.Errorf("FABRIC_CFG_PATH %s does not exist", altPath)
		}

		c.AddConfigPath(altPath)
	} else {
		// If we get here, we should use the default paths in priority order:
		//
		// *) CWD
		// *) /etc/hyperledger/fabric

		// CWD
		c.AddConfigPath("./")

		// And finally, the official path
		fi, err := os.Stat(OfficialPath)
		if err == nil && fi.IsDir() {
			c.AddConfigPath(OfficialPath)
		}
	}

	// Now set the configuration file.
	c.SetConfigName(configName)

	return nil
}

// Explicitly setting the full config file name.
// In this case, configPaths search shall not be done
func (c *ConfigParser) SetConfigFile(in string) {
	if in != "" {
		c.configFile = in
	}
}

// Return the used configFile to fill config store
func (c *ConfigParser) ConfigFileUsed() string { return c.configFile }

// Set the env variable prefix
// If "peer" is set here, all environment variable
// searches shall be prefixed with "PEER_"
func (c *ConfigParser) SetEnvPrefix(in string) {
	if in != "" {
		c.envPrefix = in
	}
}

// Search for the existence of filename for all supported extensions
func (c *ConfigParser) searchInPath(in string) (filename string) {
	logger.Debugf("Searching for config in ", in)
	for _, ext := range SupportedExts {
		fullPath := filepath.Join(in, c.configName+"."+ext)
		logger.Debugf("Checking for ", fullPath)
		_, err := os.Stat(fullPath)
		if err == nil {
			logger.Debugf("Found: ", fullPath)
			return fullPath
		}
	}
	return ""
}

// Search for the configName in all configPaths
func (c *ConfigParser) findConfigFile() string {

	logger.Debugf("Searching for config in ", c.configPaths)

	for _, cp := range c.configPaths {
		file := c.searchInPath(cp)
		if file != "" {
			return file
		}
	}
	return ""
}

// Get the valid and present config file
func (c *ConfigParser) getConfigFile() string {
	// if explicitly set, then use it
	if c.configFile != "" {
		return c.configFile
	}

	c.configFile = c.findConfigFile()
	return c.configFile
}

func (c *ConfigParser) ReadInConfig() error {

	cf := c.getConfigFile()
	logger.Debugf("Attempting to read from config file : %s", cf)
	file, err := ioutil.ReadFile(cf)
	if err != nil {
		logger.Errorf("Unable to read from config file : %s", cf)
		return err
	}

	//c.config = make(map[string]interface{})

	buf := new(bytes.Buffer)
	buf.ReadFrom(bytes.NewReader(file))
	// read the configFile in config
	return yaml.Unmarshal(buf.Bytes(), c.config)
}

// Identify the configFile from the provided configPaths
// and read and unmarshall and store in config.
func (c *ConfigParser) ReadConfig(in io.Reader) error {
	//c.config = make(map[string]interface{})
	buf := new(bytes.Buffer)
	buf.ReadFrom(in)
	return yaml.Unmarshal(buf.Bytes(), c.config)
}

// Specifies the replacement string.
// Used in searching env. variables.
func (c *ConfigParser) SetEnvKeyReplacer(r *strings.Replacer) {
	c.envKeyReplacer = r
}

// Set the environment variable search boolean
func (c *ConfigParser) AutomaticEnv() {
	c.automaticEnvApplied = true
}

// Get value for the key by searching
// environment variables
func (c *ConfigParser) GetFromEnv(key string) interface{} {
	envKey := key
	if c.envPrefix != "" {
		envKey = strings.ToUpper(c.envPrefix + "_" + envKey)
	}
	if c.envKeyReplacer != nil {
		envKey = c.envKeyReplacer.Replace(envKey)
	}
	return os.Getenv(envKey)
}

// Return the parsed config store
func (c *ConfigParser) GetParsedConfig() map[string]interface{} {
	return c.config
}

func getKeysRecursively(base string, getKey envGetter, nodeKeys map[string]interface{}, oType reflect.Type) map[string]interface{} {
	subTypes := map[string]reflect.Type{}

	if oType != nil && oType.Kind() == reflect.Struct {
	outer:
		for i := 0; i < oType.NumField(); i++ {
			fieldName := oType.Field(i).Name
			fieldType := oType.Field(i).Type

			for key := range nodeKeys {
				if strings.EqualFold(fieldName, key) {
					subTypes[key] = fieldType
					continue outer
				}
			}

			subTypes[fieldName] = fieldType
			nodeKeys[fieldName] = nil
		}
	}

	result := make(map[string]interface{})
	for key := range nodeKeys {
		fqKey := base + key

		var val interface{}
		val = nodeKeys[key]
		// overwrite val, if an environment is available
		if envVal := getKey(fqKey); envVal != "" {
			val = envVal
		}
		if m, ok := val.(map[interface{}]interface{}); ok {
			logger.Debugf("Found map[interface{}]interface{} value for %s", fqKey)
			tmp := make(map[string]interface{})
			for ik, iv := range m {
				cik, ok := ik.(string)
				if !ok {
					panic("Non string key-entry")
				}
				tmp[cik] = iv
			}
			result[key] = getKeysRecursively(fqKey+".", getKey, tmp, subTypes[key])
		} else if m, ok := val.(map[string]interface{}); ok {
			logger.Debugf("Found map[string]interface{} value for %s", fqKey)
			result[key] = getKeysRecursively(fqKey+".", getKey, m, subTypes[key])
		} else if m, ok := unmarshalJSON(val); ok {
			logger.Debugf("Found real value for %s setting to map[string]string %v", fqKey, m)
			result[key] = m
		} else {
			if val == nil {
				var fileVal interface{}
				fileSubKey := fqKey + ".File"
				if envVal := getKey(fileSubKey); envVal != "" {
					fileVal = envVal
				}
				if fileVal != nil {
					result[key] = map[string]interface{}{"File": fileVal}
					continue
				}
			}
			logger.Debugf("Found real value for %s setting to %T %v", fqKey, val, val)
			result[key] = val

		}
	}
	return result
}

func unmarshalJSON(val interface{}) (map[string]string, bool) {
	mp := map[string]string{}

	s, ok := val.(string)
	if !ok {
		logger.Debugf("Unmarshal JSON: value is not a string: %v", val)
		return nil, false
	}
	err := json.Unmarshal([]byte(s), &mp)
	if err != nil {
		logger.Debugf("Unmarshal JSON: value cannot be unmarshalled: %s", err)
		return nil, false
	}
	return mp, true
}

// customDecodeHook adds the additional functions of parsing durations from strings
// as well as parsing strings of the format "[thing1, thing2, thing3]" into string slices
// Note that whitespace around slice elements is removed
func customDecodeHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	durationHook := mapstructure.StringToTimeDurationHookFunc()
	dur, err := mapstructure.DecodeHookExec(durationHook, f, t, data)
	if err == nil {
		if _, ok := dur.(time.Duration); ok {
			return dur, nil
		}
	}

	if f.Kind() != reflect.String {
		return data, nil
	}

	raw := data.(string)
	l := len(raw)
	if l > 1 && raw[0] == '[' && raw[l-1] == ']' {
		slice := strings.Split(raw[1:l-1], ",")
		for i, v := range slice {
			slice[i] = strings.TrimSpace(v)
		}
		return slice, nil
	}

	return data, nil
}

func byteSizeDecodeHook(f reflect.Kind, t reflect.Kind, data interface{}) (interface{}, error) {
	if f != reflect.String || t != reflect.Uint32 {
		return data, nil
	}
	raw := data.(string)
	if raw == "" {
		return data, nil
	}
	var re = regexp.MustCompile(`^(?P<size>[0-9]+)\s*(?i)(?P<unit>(k|m|g))b?$`)
	if re.MatchString(raw) {
		size, err := strconv.ParseUint(re.ReplaceAllString(raw, "${size}"), 0, 64)
		if err != nil {
			return data, nil
		}
		unit := re.ReplaceAllString(raw, "${unit}")
		switch strings.ToLower(unit) {
		case "g":
			size = size << 10
			fallthrough
		case "m":
			size = size << 10
			fallthrough
		case "k":
			size = size << 10
		}
		if size > math.MaxUint32 {
			return size, fmt.Errorf("value '%s' overflows uint32", raw)
		}
		return size, nil
	}
	return data, nil
}

func stringFromFileDecodeHook(f reflect.Kind, t reflect.Kind, data interface{}) (interface{}, error) {
	// "to" type should be string
	if t != reflect.String {
		return data, nil
	}
	// "from" type should be map
	if f != reflect.Map {
		return data, nil
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.String:
		return data, nil
	case reflect.Map:
		d := data.(map[string]interface{})
		fileName, ok := d["File"]
		if !ok {
			fileName, ok = d["file"]
		}
		switch {
		case ok && fileName != nil:
			bytes, err := ioutil.ReadFile(fileName.(string))
			if err != nil {
				return data, err
			}
			return string(bytes), nil
		case ok:
			// fileName was nil
			return nil, fmt.Errorf("Value of File: was nil")
		}
	}
	return data, nil
}

func pemBlocksFromFileDecodeHook(f reflect.Kind, t reflect.Kind, data interface{}) (interface{}, error) {
	// "to" type should be string
	if t != reflect.Slice {
		return data, nil
	}
	// "from" type should be map
	if f != reflect.Map {
		return data, nil
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.String:
		return data, nil
	case reflect.Map:
		var fileName string
		var ok bool
		switch d := data.(type) {
		case map[string]string:
			fileName, ok = d["File"]
			if !ok {
				fileName, ok = d["file"]
			}
		case map[string]interface{}:
			var fileI interface{}
			fileI, ok = d["File"]
			if !ok {
				fileI = d["file"]
			}
			fileName, ok = fileI.(string)
		}

		switch {
		case ok && fileName != "":
			var result []string
			bytes, err := ioutil.ReadFile(fileName)
			if err != nil {
				return data, err
			}
			for len(bytes) > 0 {
				var block *pem.Block
				block, bytes = pem.Decode(bytes)
				if block == nil {
					break
				}
				if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
					continue
				}
				result = append(result, string(pem.EncodeToMemory(block)))
			}
			return result, nil
		case ok:
			// fileName was nil
			return nil, fmt.Errorf("Value of File: was nil")
		}
	}
	return data, nil
}

var kafkaVersionConstraints map[sarama.KafkaVersion]version.Constraints

func init() {
	kafkaVersionConstraints = make(map[sarama.KafkaVersion]version.Constraints)
	kafkaVersionConstraints[sarama.V0_8_2_0], _ = version.NewConstraint(">=0.8.2,<0.8.2.1")
	kafkaVersionConstraints[sarama.V0_8_2_1], _ = version.NewConstraint(">=0.8.2.1,<0.8.2.2")
	kafkaVersionConstraints[sarama.V0_8_2_2], _ = version.NewConstraint(">=0.8.2.2,<0.9.0.0")
	kafkaVersionConstraints[sarama.V0_9_0_0], _ = version.NewConstraint(">=0.9.0.0,<0.9.0.1")
	kafkaVersionConstraints[sarama.V0_9_0_1], _ = version.NewConstraint(">=0.9.0.1,<0.10.0.0")
	kafkaVersionConstraints[sarama.V0_10_0_0], _ = version.NewConstraint(">=0.10.0.0,<0.10.0.1")
	kafkaVersionConstraints[sarama.V0_10_0_1], _ = version.NewConstraint(">=0.10.0.1,<0.10.1.0")
	kafkaVersionConstraints[sarama.V0_10_1_0], _ = version.NewConstraint(">=0.10.1.0,<0.10.2.0")
	kafkaVersionConstraints[sarama.V0_10_2_0], _ = version.NewConstraint(">=0.10.2.0,<0.11.0.0")
	kafkaVersionConstraints[sarama.V0_11_0_0], _ = version.NewConstraint(">=0.11.0.0,<1.0.0")
	kafkaVersionConstraints[sarama.V1_0_0_0], _ = version.NewConstraint(">=1.0.0")
}

func kafkaVersionDecodeHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f.Kind() != reflect.String || t != reflect.TypeOf(sarama.KafkaVersion{}) {
		return data, nil
	}

	v, err := version.NewVersion(data.(string))
	if err != nil {
		return nil, fmt.Errorf("Unable to parse Kafka version: %s", err)
	}

	for kafkaVersion, constraints := range kafkaVersionConstraints {
		if constraints.Check(v) {
			return kafkaVersion, nil
		}
	}

	return nil, fmt.Errorf("Unsupported Kafka version: '%s'", data)
}

func bccspHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if t != reflect.TypeOf(&factory.FactoryOpts{}) {
		return data, nil
	}

	config := factory.GetDefaultOpts()

	err := mapstructure.WeakDecode(data, config)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode bcssp type")
	}

	return config, nil
}

// EnhancedExactUnmarshal is intended to unmarshal a config file into a structure
// producing error when extraneous variables are introduced and supporting
// the time.Duration type
func EnhancedExactUnmarshal(c *ConfigParser, output interface{}) error {
	oType := reflect.TypeOf(output)
	if oType.Kind() != reflect.Ptr {
		return errors.Errorf("supplied output argument must be a pointer to a struct but is not pointer")
	}
	eType := oType.Elem()
	if eType.Kind() != reflect.Struct {
		return errors.Errorf("supplied output argument must be a pointer to a struct, but it is pointer to something else")
	}

	baseKeys := c.GetParsedConfig()
	getterWithClass := func(key string) interface{} { return c.GetFromEnv(key) }
	leafKeys := getKeysRecursively("", getterWithClass, baseKeys, eType)

	logger.Debugf("%+v", leafKeys)
	config := &mapstructure.DecoderConfig{
		ErrorUnused:      true,
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			bccspHook,
			customDecodeHook,
			byteSizeDecodeHook,
			stringFromFileDecodeHook,
			pemBlocksFromFileDecodeHook,
			kafkaVersionDecodeHook,
		),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	return decoder.Decode(leafKeys)
}
