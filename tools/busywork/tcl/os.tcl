# os.tcl - Utilities related to the operating system functions

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

package require Tclx

############################################################################
# waitPIDs pids {digest {}}

# Wait for a list of PIDs to finish. Return 0 if all subprocesses exit
# normally, -1 otherwise. The caller may also provide an optional 'digest'
# array. This array variable will be populated with key/values

#     digest($pid.why) <- EXIT, SIG, STOP, other
#     digest($pid.rc)  <- Process return code
#     digest($pid.ok)  <- why == EXIT && rc == 0

proc waitPIDs {pids {digest {}}} {

    if {![null digest]} {
        upvar $digest _digest
    } else {
        array unset _digest
    }

    set rv 0
    foreach pid $pids {

        foreach {pid why rc} [wait $pid] break

        set _digest($pid.why) $why
        set _digest($pid.rc) $rc
        set _digest($pid.ok) [expr {($why eq "EXIT") && ($rc == 0)}]
        
        switch $why {
            EXIT {
                if {$rc != 0} {
                    err {} "Process $pid exited with rc = $rc : Failure"
                    set rv -1
                }
            }
            SIG {
                err {} "Process $pid caught signal $rc : Failure"
                set rv -1
            }
            STOP {
                err {} "Process $pid is stopped(???) : Failure"
                set rv -1
            }
            default {
                err {} "Unexpected return from wait : $pid $why $rc : Failure"
                set rv -1
            }
        }
    }
    return $rv
}
