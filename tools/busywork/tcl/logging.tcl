# logging.tcl - "Module-level" logging

# Copyright IBM Corp. 2016. All Rights Reserved.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
# 		 http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# There are 4 logging levels: DEBUG, NOTE, WARN and ERROR. Presumably errors
# and above will usually use errorExit, but sometimes it is necessary to
# format long error messages. The NOTE level is used for "normal" messages and
# is not tagged with a level on output.

# Logging levels are selectable by "module", which is simply an
# application-specific string. If no module-specific level has been set, then
# the default level "" is used. Module-specific levels are set by the
# setLoggingLevel{} procedure. There are also pre-defined modules called
# debug, note, warn and err which are pre-initialized to log at that level,
# e.g., [note note "Message"] will always print "Message" at NOTE level.

# The setLoggingPrefix{} procedure sets a prefix that will be prepended to all
# logged messages. The default is the empty prefix. The setLoggingTimestamp{}
# procedure can be used to enable timestamps to be prepended to logs. The
# default is not to timestamp the logs.

set ::LOG_LEVEL_DEBUG 0
set ::LOG_LEVEL_NOTE  1
set ::LOG_LEVEL_WARN  2
set ::LOG_LEVEL_ERROR 3

array unset ::moduleLoggingLevel
set ::moduleLoggingLevel() $::LOG_LEVEL_WARN; # There is no such a({})!

proc setLoggingLevel {i_module i_level} {

    set level [string tolower $i_level]
    switch $level {
        0 -
        debug {
            set level $::LOG_LEVEL_DEBUG
        }
        1 -
        note {
            set level $::LOG_LEVEL_NOTE
        }            
        2 -
        warn {
            set level $::LOG_LEVEL_WARN
        }
        3 -
        "error" {
            set level $::LOG_LEVEL_ERROR
        }
        default {
            errorExit "Invalid logging level '$i_level'"
        }
    }

    set ::moduleLoggingLevel($i_module) $level
}

setLoggingLevel debug debug
setLoggingLevel note  note
setLoggingLevel warn  warn
setLoggingLevel err   error

proc setLoggingPrefix {i_prefix} {

    if {$i_prefix eq {}} {
        foreach level {DEBUG WARNING ERROR} {
            set ::loggingPrefix($level) [list "$level: "]
        }
        set ::loggingPrefix(NOTE) ""
    } else {
        foreach level {DEBUG WARNING ERROR} {
            set ::loggingPrefix($level) [list "$i_prefix:$level: "]
        }
        set ::loggingPrefix(NOTE) [list "$i_prefix: "]
    }
}

setLoggingPrefix {}

set ::loggingTimestamp 0

proc setLoggingTimestamp {bool} {

    if {$bool} {} {};           # Error check
    set ::loggingTimestamp $bool
}

proc debug {i_module args} {
    moduleLevelLogging $i_module DEBUG $::LOG_LEVEL_DEBUG $args
}

proc note {i_module args} {
    moduleLevelLogging $i_module NOTE $::LOG_LEVEL_NOTE $args
}

proc warn {i_module args} {
    moduleLevelLogging $i_module WARNING $::LOG_LEVEL_WARN $args
}

proc err {i_module args} {
    moduleLevelLogging $i_module ERROR $::LOG_LEVEL_ERROR $args
}

proc moduleLevelLogging {i_module i_sLevel i_nLevel i_args} {
    if {([info exists ::moduleLoggingLevel($i_module)] &&
         ($::moduleLoggingLevel($i_module) <= $i_nLevel)) ||
        (![info exists ::moduleLoggingLevel($i_module)] &&
           ($::moduleLoggingLevel() <= $i_nLevel))} {

        if {$::loggingTimestamp} {
            
            eval putsx -stream stderr \
                [timestamp -hybrid] {" "} \
                $::loggingPrefix($i_sLevel) $i_args

        } else {
            
            eval putsx -stream stderr \
                $::loggingPrefix($i_sLevel) $i_args
        }
    }
}
