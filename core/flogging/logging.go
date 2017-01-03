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
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// A logger to log logging logs!
var loggingLogger = logging.MustGetLogger("logging")

var loggingDefaultFormat = "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"
var loggingDefaultOutput = os.Stderr

// The default logging level, in force until LoggingInit() is called or in
// case of configuration errors.
var loggingDefaultLevel = logging.INFO

// LoggingInit is a 'hook' called at the beginning of command processing to
// parse logging-related options specified either on the command-line or in
// config files.  Command-line options take precedence over config file
// options, and can also be passed as suitably-named environment variables. To
// change module logging levels at runtime call `logging.SetLevel(level,
// module)`.  To debug this routine include logging=debug as the first
// term of the logging specification.
func LoggingInit(command string) {
	// Parse the logging specification in the form
	//     [<module>[,<module>...]=]<level>[:[<module>[,<module>...]=]<level>...]
	defaultLevel := loggingDefaultLevel
	var err error
	spec := viper.GetString("logging_level")
	if spec == "" {
		spec = viper.GetString("logging." + command)
	}
	if spec != "" {
		fields := strings.Split(spec, ":")
		for _, field := range fields {
			split := strings.Split(field, "=")
			switch len(split) {
			case 1:
				// Default level
				defaultLevel, err = logging.LogLevel(field)
				if err != nil {
					loggingLogger.Warningf("Logging level '%s' not recognized, defaulting to %s : %s", field, loggingDefaultLevel, err)
					defaultLevel = loggingDefaultLevel // NB - 'defaultLevel' was overwritten
				}
			case 2:
				// <module>[,<module>...]=<level>
				if level, err := logging.LogLevel(split[1]); err != nil {
					loggingLogger.Warningf("Invalid logging level in '%s' ignored", field)
				} else if split[0] == "" {
					loggingLogger.Warningf("Invalid logging override specification '%s' ignored - no module specified", field)
				} else {
					modules := strings.Split(split[0], ",")
					for _, module := range modules {
						logging.SetLevel(level, module)
						loggingLogger.Debugf("Setting logging level for module '%s' to %s", module, level)
					}
				}
			default:
				loggingLogger.Warningf("Invalid logging override '%s' ignored; Missing ':' ?", field)
			}
		}
	}
	// Set the default logging level for all modules
	logging.SetLevel(defaultLevel, "")
	loggingLogger.Debugf("Setting default logging level to %s for command '%s'", defaultLevel, command)
}

// DefaultLoggingLevel returns the fallback value for loggers to use if parsing fails
func DefaultLoggingLevel() logging.Level {
	return loggingDefaultLevel
}

// Initiate 'leveled' logging using the default format and output location
func init() {
	SetLoggingFormat(loggingDefaultFormat, loggingDefaultOutput)
}

// SetLoggingFormat sets the logging format and the location of the log output
func SetLoggingFormat(formatString string, output io.Writer) {
	if formatString == "" {
		formatString = loggingDefaultFormat
	}
	format := logging.MustStringFormatter(formatString)

	initLoggingBackend(format, output)
}

// initialize the logging backend based on the provided logging formatter
// and I/O writer
func initLoggingBackend(logFormatter logging.Formatter, output io.Writer) {
	backend := logging.NewLogBackend(output, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, logFormatter)
	logging.SetBackend(backendFormatter).SetLevel(loggingDefaultLevel, "")
}

// GetModuleLogLevel gets the current logging level for the specified module
func GetModuleLogLevel(module string) (string, error) {
	// logging.GetLevel() returns the logging level for the module, if defined.
	// otherwise, it returns the default logging level, as set by
	// flogging/logging.go
	level := logging.GetLevel(module).String()

	loggingLogger.Debugf("Module '%s' logger enabled for log level: %s", module, level)

	return level, nil
}

// SetModuleLogLevel sets the logging level for the specified module. This is
// currently only called from admin.go but can be called from anywhere in the
// code on a running peer to dynamically change the log level for the module.
func SetModuleLogLevel(module string, logLevel string) (string, error) {
	level, err := logging.LogLevel(logLevel)

	if err != nil {
		loggingLogger.Warningf("Invalid logging level: %s - ignored", logLevel)
	} else {
		logging.SetLevel(logging.Level(level), module)
		loggingLogger.Debugf("Module '%s' logger enabled for log level: %s", module, level)
	}

	logLevelString := level.String()

	return logLevelString, err
}
