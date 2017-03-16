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

package flogging

import (
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// A logger to log logging logs!
var logger = logging.MustGetLogger("logging")

var defaultFormat = "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"
var defaultOutput = os.Stderr

// The default logging level, in force until LoggingInit() is called or in
// case of configuration errors.
var fallbackDefaultLevel = logging.INFO

var lock = sync.Mutex{}

// modules holds the map of all modules and the current log level
var modules map[string]string

// IsSetLevelByRegExpEnabled disables the code that will allow setting log
// levels using a regular expression instead of one module/submodule at a time.
// TODO - remove once peer code has switched over to using
// flogging.MustGetLogger in place of logging.MustGetLogger
var IsSetLevelByRegExpEnabled = false

func init() {
	modules = make(map[string]string)
}

// InitFromViper is a 'hook' called at the beginning of command processing to
// parse logging-related options specified either on the command-line or in
// config files.  Command-line options take precedence over config file
// options, and can also be passed as suitably-named environment variables. To
// change module logging levels at runtime call `logging.SetLevel(level,
// module)`.  To debug this routine include logging=debug as the first
// term of the logging specification.
// TODO this initialization is specific to the peer config format.  The viper
// references should be removed, and this path should be moved into the peer
func InitFromViper(command string) {
	spec := viper.GetString("logging_level")
	if spec == "" {
		spec = viper.GetString("logging." + command)
	}
	defaultLevel := InitFromSpec(spec)
	logger.Debugf("Setting default logging level to %s for command '%s'", defaultLevel, command)
}

// InitFromSpec initializes the logging based on the supplied spec, it is exposed externally
// so that consumers of the flogging package who do not wish to use the default config structure
// may parse their own logging specification
func InitFromSpec(spec string) logging.Level {
	// Parse the logging specification in the form
	//     [<module>[,<module>...]=]<level>[:[<module>[,<module>...]=]<level>...]
	defaultLevel := fallbackDefaultLevel
	var err error
	if spec != "" {
		fields := strings.Split(spec, ":")
		for _, field := range fields {
			split := strings.Split(field, "=")
			switch len(split) {
			case 1:
				// Default level
				defaultLevel, err = logging.LogLevel(field)
				if err != nil {
					logger.Warningf("Logging level '%s' not recognized, defaulting to %s : %s", field, defaultLevel, err)
					defaultLevel = fallbackDefaultLevel // NB - 'defaultLevel' was overwritten
				}
			case 2:
				// <module>[,<module>...]=<level>
				if level, err := logging.LogLevel(split[1]); err != nil {
					logger.Warningf("Invalid logging level in '%s' ignored", field)
				} else if split[0] == "" {
					logger.Warningf("Invalid logging override specification '%s' ignored - no module specified", field)
				} else {
					modules := strings.Split(split[0], ",")
					for _, module := range modules {
						logging.SetLevel(level, module)
						logger.Debugf("Setting logging level for module '%s' to %s", module, level)
					}
				}
			default:
				logger.Warningf("Invalid logging override '%s' ignored; Missing ':' ?", field)
			}
		}
	}
	// Set the default logging level for all modules
	logging.SetLevel(defaultLevel, "")
	return defaultLevel
}

// DefaultLevel returns the fallback value for loggers to use if parsing fails
func DefaultLevel() logging.Level {
	return fallbackDefaultLevel
}

// Initiate 'leveled' logging using the default format and output location
func init() {
	SetLoggingFormat(defaultFormat, defaultOutput)
}

// SetLoggingFormat sets the logging format and the location of the log output
func SetLoggingFormat(formatString string, output io.Writer) {
	if formatString == "" {
		formatString = defaultFormat
	}
	format := logging.MustStringFormatter(formatString)

	initLoggingBackend(format, output)
}

// initialize the logging backend based on the provided logging formatter
// and I/O writer
func initLoggingBackend(logFormatter logging.Formatter, output io.Writer) {
	backend := logging.NewLogBackend(output, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, logFormatter)
	logging.SetBackend(backendFormatter).SetLevel(fallbackDefaultLevel, "")
}

// GetModuleLevel gets the current logging level for the specified module
func GetModuleLevel(module string) (string, error) {
	// logging.GetLevel() returns the logging level for the module, if defined.
	// otherwise, it returns the default logging level, as set by
	// flogging/logging.go
	level := logging.GetLevel(module).String()

	logger.Debugf("Module '%s' logger enabled for log level: %s", module, level)

	return level, nil
}

// SetModuleLevel sets the logging level for the modules that match the
// supplied regular expression. This is can be called from anywhere in the
// code on a running peer to dynamically change log levels.
func SetModuleLevel(moduleRegExp string, logLevel string) (string, error) {
	level, err := logging.LogLevel(logLevel)

	if err != nil {
		logger.Warningf("Invalid logging level: %s - ignored", logLevel)
	} else {
		// TODO - this check is in here temporarily until all modules have been
		// converted to using flogging.MustGetLogger. until that point, this flag
		// keeps the previous behavior in place.
		if !IsSetLevelByRegExpEnabled {
			logging.SetLevel(logging.Level(level), moduleRegExp)
			logger.Infof("Module '%s' logger enabled for log level: %s", moduleRegExp, level)
		} else {
			re, err := regexp.Compile(moduleRegExp)
			if err != nil {
				logger.Warningf("Invalid regular expression for module: %s", moduleRegExp)
				return "", err
			}

			lock.Lock()
			defer lock.Unlock()
			for module := range modules {
				if re.MatchString(module) {
					logging.SetLevel(logging.Level(level), module)
					modules[module] = logLevel
					logger.Infof("Module '%s' logger enabled for log level: %s", module, level)
				}
			}
		}
	}

	logLevelString := level.String()

	return logLevelString, err
}

// MustGetLogger is used in place of logging.MustGetLogger to allow us to store
// a map of all modules and submodules that have loggers in the system
func MustGetLogger(module string) *logging.Logger {
	logger := logging.MustGetLogger(module)

	// retrieve the current log level for the logger
	level := logging.GetLevel(module).String()

	// store the module's name as well as the current log level for the logger
	lock.Lock()
	defer lock.Unlock()
	modules[module] = level

	return logger
}
