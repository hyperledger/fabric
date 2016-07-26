# atExit.tcl - Exit Handlers

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

# This is a simple atExit() facility for Tcl.  Exit handlers are registered
# with atExit{} and atErrorExit{}, and will be run in LIFO (stack)
# order. Handlers registered with atErrorExit{} are only run if the exit code
# is non-0, but maintain the LIFO order with universal handlers. The exit{}
# command is redefined to run the handlers, then call the original Tcl exit{}
# once done. If any of the exit handlers fail the script will fail. The
# clearAtExitHandlers{} command is provided for subprocesses that may be
# fork()-ed from a parent.

# Note that if any of the exit handlers call exit{} themselves, the script
# will immediately terminate with that exit code due to the redefinition of
# exit{}. 

# atExit logging is controlled as the module 'atExit', and defaults to ERROR.

setLoggingLevel atExit error

set ::atExitHandlers {}

proc atExit {handler} {
    lappend ::atExitHandlers [list 0 $handler]
}

proc atErrorExit {handler} {
    lappend ::atExitHandlers [list 1 $handler]
}

proc clearAtExitHandlers {} {
    set ::atExitHandlers {}
}

rename exit theRealExit

proc exit {args} {

    rename exit ""

    proc ::exit {args} {
        eval uplevel theRealExit $args
    }

    set errors 0
    set errorExit [expr {[first $args] != 0}]
    foreach pair [lreverse $::atExitHandlers] {
        foreach {errorOnly handler} $pair break
        if {!$errorOnly || $errorExit} {
            if {[catch [list uplevel #0 $handler]]} {
                set errors 1
                err err "atExit handler failed; Script will fail with exit -1"
                puts stderr $::errorInfo
            }
        }
    }

    if {$errors} {

        exit -1

    } else {

        eval exit $args
    }
}


# Register an atExit handler to safely kill a PID at exit

proc killAtExit {signal pid} {

    atExit [list catch [list atExitKiller $signal $pid]]
}

proc atExitKiller {signal pid} {

    package require Tclx

    note atExit "atExitKiller : Killing $pid"
    kill $signal $pid
}
