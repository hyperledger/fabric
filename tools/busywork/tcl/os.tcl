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


############################################################################
# procPIDStat i_pid o_array {i_prefix ""}

# This procedure parses the result of executing

#     cat /proc/[i_pid]/stat

# and stores the results in the array o_array keyed by name. See man proc(5)
# under /proc/[pid]/stat for an interpretation of the fields. The names are
# taken directly from the man page. The optional i_prefix can be specified to
# allow stats for multiple PIDs to be stored in the same array, using an
# indexing scheme chosen by the caller.

# NB: This procedure only works on Linux, and will fail on other operating
# systems.

# BUGS: Field 2 (comm) is the command name in parenthesis. If the file name of
# the command includes white space this will throw off the current parser.

# Note: Copy/edited taken directly from the man page, which uses 1-based
# addressing.
array unset ::procPIDStatMap
foreach {index key} {
    1 pid
    2 comm
    3 state
    4 ppid
    5 pgrp
    6 session
    7 tty_nr
    8 tpgid
    9 flags
    10 minflt
    11 cminflt
    12 majflt
    13 cmajflt
    14 utime
    15 stime
    16 cutime
    17 cstime
    18 priority
    19 nice
    20 num_threads
    21 itrealvalue
    22 starttime
    23 vsize
    24 rss
    25 rsslim
    26 startcode
    27 endcode
    28 startstack
    29 kstkesp
    30 kstkeip
    31 signal
    32 blocked
    33 sigignore
    34 sigcatch
    35 wchan
    36 nswap
    37 cnswap
    38 exit_signal
    39 processor
    40 rt_priority
    41 policy
    42 delayacct_blkio_ticks
    43 guest_time
    44 cguest_time
    45 start_data
    46 end_data
    47 start_brk
    48 arg_start
    49 arg_end
    50 env_start
    51 env_end
    52 exit_code
} {
    set ::procPIDStatMap([expr {$index - 1}]) $key
}


proc procPIDStat {i_pid o_array {i_prefix ""}} {

    upvar $o_array a

    if {[catch {exec cat /proc/$i_pid/stat} stat]} {
        errorExit "Can not cat /proc/$i_pid/stat: $stat"
    }

    foreach index [array names ::procPIDStatMap] {
        set a($i_prefix$::procPIDStatMap($index)) [lindex $stat $index]
    }
}
