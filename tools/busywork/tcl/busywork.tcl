# busywork.tcl - General support for busywork applications

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

# Many of these procedures require that the BUSYWORK_HOME environment variable
# is set. Therefore the ::busywork::home procedure is run whenever the package
# is loaded. Loading the 'busywork' package also loads all other packages
# required by busywork scripts.

package require fabric
package require json
package require json::write
package require math
package require Tclx
package require utils
package require yaml

package provide busywork 0.0
namespace eval ::busywork {}

############################################################################
# ::busywork::home

# If the BUSYWORK_HOME environment variable is not given or defined, set it to
# $HOME/.busywork and ensure that the directory exists. The final value of
# $BUSYWORK_HOME is returned.

proc ::busywork::home {{i_home {}}} {
    if {![null $i_home]} {
        set ::env(BUSYWORK_HOME) $i_home
    } elseif {![info exists ::env(BUSYWORK_HOME)] ||
                [null $::env(BUSYWORK_HOME)]} {
        set ::env(BUSYWORK_HOME) $::env(HOME)/.busywork
    }
    if {[catch {exec mkdir -p $::env(BUSYWORK_HOME)} why]} {
        errorExit "Attempt to create $::env(BUSYWORK_HOME) failed : $why"
    }
    return $::env(BUSYWORK_HOME)
}

::busywork::home


############################################################################
# ::busywork::fabric
# ::busywork::busywork
# ::busywork::bin

# The busywork::fabric procedure deconstructs the GOPATH and returns the path
# to the first occurrence of github.com/hyperledger/fabric. The
# busywork::busywork procedure locates the busywork directory, and
# busywork::bin locates the busywork/bin directory.

proc ::busywork::fabric {} {
    foreach dir [split $::env(GOPATH)] {
        set fabric $dir/src/github.com/hyperledger/fabric
        if {[file exists $fabric]} {
            return $fabric
        }
    }
    errorExit\
        "Could not find an element of the GOPATH " \
        "that includes the Hyperledger fabric"
}


proc ::busywork::busywork {} {
    return [fabric]/tools/busywork
}


proc ::busywork::bin {} {
    return [busywork]/bin
}


############################################################################
# ::busywork::networkToArray i_array {i_prefix {}}

# Load and parse the $BUSYWORK_HOME/network file into keys in the i_array
# provided by the caller. The procedure uses the normal Tcl 'error' exit only
# in the event of being unable to read the network file. Other errors invoke
# the 'errorExit' procedure.

# This procedure uses "." as a separator for hierarchical keys, so normally
# the optional prefix should also end in ".".

# This routine is typically called from busywork scripts as

#     busywork::networkToArray ::parms network.

# to add network parameterization to the global ::parms array.  In the following
# we assume networkToArray has been called like this to illustrate its
# operation.

# All of the top-level keys of the JSON object are inserted as-is. For the
# peers, keys are created that list all of the peer fields in order (with a
# long but grammatical name), e.g.,

#     network.peer.restAddresses, network.peer.grpcAddresses, ...

# Individual elements of each peer are also directly addressible by the peer
# ID, e.g.,

#     network.peer.umvp0.rest, network.peer.umvp1.grpc, ...

# Since there is only one membersrvc server, its keys are singular:

#     network.membersrvc.service, network.membersrvc.profile, ...

proc ::busywork::networkToArray {i_array {i_prefix {}}} {

    upvar $i_array a

    set networkFile $::env(BUSYWORK_HOME)/network
    if {[catch {open $networkFile r} network]} {
        error "Can't open $networkFile for reading : $network"
    }
    if {[catch {::json::json2dict [read $network]} dict]} {
        errorExit "Error parsing $networkFile : $::errorCode"
    }
    close $network

    # Do the top-level keys

    foreach {key val} $dict {
        set a($i_prefix$key) $val
    }

    # Do the peer keys

    set plurals {
        id ids
        grpc grpcAddresses
        rest restAddresses
        events eventsAddresses
        profile profileAddresses
        pid pids
    }

    foreach peer [dict get $dict peers] {
        foreach {key plural} $plurals {
            set val [dict get $peer $key]
            set id [dict get $peer id]
            lappend a($i_prefix\peer.$plural) $val
            set a($i_prefix\peer.$id.$key) $val
        }
    }

    # Do the membersrvc keys

    if {$a($i_prefix\security) eq "true"} {
        set membersrvc [dict get $dict membersrvc]
        foreach key [dict keys $membersrvc] {
            set a($i_prefix\membersrvc.$key) [dict get $membersrvc $key]
        }
    }
}


############################################################################
# ::busywork::chaincodesToArray i_array {i_prefix {}}

# Load and parse the $BUSYWORK_HOME/chaincodes file into keys in the i_array
# provided by the caller. The procedure uses the normal Tcl 'error' exit only
# in the event of being unable to read the network file. Other errors invoke
# the 'errorExit' procedure.

# This procedure uses "." as a separator for hierarchical keys, so normally
# the optional prefix should also end in ".".

# This routine is typically called from busywork scripts as

#     busywork::chaincodesToArray ::parms chaincodes.

# to add chaincode data to the global ::parms array.  In the following we
# assume chaincodesToArray has been called like this to illustrate its
# operation.

# Entries in the chaincodes file are tagged with a user-given chaincode
# ID. A list of all of these IDs will be stored as the 'chaincodes.ids'
# key. For each ID $id in that list, the following keys will also be stored:
#
#     chaincodes.$id.name     # Chaincode name (hash)
#     chaincodes.$id.path     # Chaincode path
#     chaincodes.$id.function # Chaincode initialization function
#     chaincodes.$id.parms    # Chaincode initialization parameters
#
# Lists of all names, paths etc. will also be stored as the keys
#
#     chaincodes.name chaincodes.path chaincodes.function chaincodes.parms

proc ::busywork::chaincodesToArray {i_array {i_prefix {}}} {

    upvar $i_array a

    set chaincodesFile $::env(BUSYWORK_HOME)/chaincodes
    if {[catch {open $chaincodesFile r} chaincodes]} {
        error "Can't open $chaincodesFile for reading : $chaincodes"
    }
    if {[catch {::json::json2dict [read $chaincodes]} dict]} {
        errorExit "Error parsing $chaincodesFile : $::errorCode"
    }
    close $chaincodes

    # The top-level keys are the user chaincode IDs

    foreach {id struct} $dict {

        lappend a($i_prefix\ids) $id

        foreach {key val} $struct {
            set a($i_prefix$id.$key) $val
            lappend a($i_prefix$key) $val
        }
    }
}


############################################################################
# ::busywork::addChaincode i_id i_name i_path i_function i_args

# Rewrite the JSON-formatted $BUSYWORK_HOME/chaincodes file based on a new
# chaincode deployment. If the file does not exist it will be created. The
# optional i_lockfile is a file to use as a lock on the chaincodes file, and
# we will wait forever for the lock, polling every second.

set ::busywork::CHAINCODE_FIELDS {}
array unset ::busywork::CHAINCODE_FIELD_IS_ARRAY
foreach {field isArray} {name 0 path 0 function 0 args 1} {
    lappend ::busywork::CHAINCODE_FIELDS $field
    set ::busywork::CHAINCODE_FIELD_IS_ARRAY($field) $isArray
}

proc ::busywork::addChaincode \
    {i_id i_name i_path i_function i_args {i_lockFile {}}} {

    ::json::write indented 1
    ::json::write aligned 1

    # If the file is extant, read it into the array 'a'. Otherwise initialize
    # 'a'. 

    set file $::env(BUSYWORK_HOME)/chaincodes
    if {[file exists $file]} {
        if {[catch {chaincodesToArray a ""} why]} {
            errorExit "$file exists but can't be accessed: $why"
        }
    } else {
        set a(ids) {}
    }

    # Add the new information

    lappend a(ids) $i_id
    foreach field $busywork::CHAINCODE_FIELDS {
        set a($i_id.$field) [set i_${field}]
    }

    # Reformat as JSON. Need to work hard to maintain double quotes in arrays.
    
    set top {}
    foreach id $a(ids) {
        set struct {}
        foreach field $busywork::CHAINCODE_FIELDS {
            if {$::busywork::CHAINCODE_FIELD_IS_ARRAY($field)} {
                lappend struct $field \
                    [eval ::json::write array \
                         [mapeach x $a($id.$field) {
                             return "\"$x\""
                         }]]
            } else {
                lappend struct $field [::json::write string $a($id.$field)]
            }
        }
        lappend top $id [eval ::json::write object $struct]
    }

    # (Re-)create the file

    if {[catch {open $file w} stream]} {
        errorExit "Can't open $file for writing: $stream"
    }
    puts $stream [eval ::json::write object $top]
    close $stream
}


############################################################################
# ::busywork::usersAndPasswordsToArray i_array {i_prefix {}}

# Load and parse the fabric/membersrvc/membersrvc.yaml file into keys in the
# i_array provided by the caller. This procedure uses "." as a separator for
# hierarchical keys, so normally the optional prefix should also end in ".".

# This routine is typically called from busywork scripts as

#     busywork::usersAndPasswordsToArray ::parms security.

# to add security parameterization to the global parms array.  In the
# following we assume usersAndPassworssToArray has been called like this to
# illustrate its operation. The following keys are created:

# security.users                       - List of all users
# security.user.<user>                 - All credentials for a <user>
# security.user.<user>.role            - User role
# security.user.<user>.password        - User password
# security.user.<user>.affiliation     - User affiliation (may be NULL)
# security.user.<user>.affiliationRole - User affiliation role (may be NULL)

proc ::busywork::usersAndPasswordsToArray {i_array {i_prefix {}}} {

    upvar $i_array a

    if {[null $i_prefix]} {
        set prefix {}
    } else {
        set prefix $i_prefix.
    }

    set yamlFile [fabric]/membersrvc/membersrvc.yaml
    if {[catch {open $yamlFile r} yaml]} {
        errorExit \
            "The security configuration file $yamlFile " \
            "could not be opened for reading : $::errorCode"
    }
    if {[catch {::yaml::yaml2dict [read $yaml]} dict]} {
        errorExit "Error parsing $yamlFile : $::errorCode"
    }
    close $yaml

    if {[catch {dict get [dict get $dict eca] users} users]} {
        errorExit "Error parsing $yamlFile : $::errorCode"
    }

    foreach {user values} $users {

        foreach {role password affiliation affiliationRole} $values break
        
        lappend a($i_prefix\users) $user
        lappend a($i_prefix\user.$user.role) $role
        lappend a($i_prefix\user.$user.password) $password
        lappend a($i_prefix\user.$user.affiliation) $affiliation
        lappend a($i_prefix\user.$user.affiliationRole) $affiliationRole
    }
}

############################################################################
# ::busywork:wait-for-it i_address i_timeout

# Wait up to i_timeout (a duration, minimum unit 1 second) for an ip address
# to become active, returning 1 if it is active and 0 otherwise.

proc ::busywork::wait-for-it {i_address i_timeout} {

    set timeout [math::max 1 [durationToIntegerSeconds $i_timeout]]
    if {[catch {exec [bin]/wait-for-it $i_address -t $timeout -q}]} {
        return 0
    } else {
        return 1
    }
}


############################################################################
# ::busywork::dockerEndpoint {i_tls 0}

# Determine the IP address and port of the Docker HTTP API.  If the
# environment includes the variable DOCKER_ENDPOINT, that value is
# returned. Otherwise this routine examines the network setup of the host and
# tries to locate the IP address of a single Docker bridge. If this fails for
# any reason we have to punt and tell the user to define DOCKER_ENDPOINT. We
# assume that Docker has been set up on port 2375 unless i_tls is true, in
# which case we use 2376.

proc ::busywork::dockerEndpoint {{i_tls 0}} {

    if {[info exists ::env(DOCKER_ENDPOINT)] &&
        ($::env(DOCKER_ENDPOINT) ne "")} {
        return $::env(DOCKER_ENDPOINT)
    }

    if {[catch {exec ip -4 -o address | grep docker} lines]} {
        err err "Error trying to locate the Docker bridge."
        err err "Check to make sure Docker is running and configured properly."
        err err \
            "Otherwise, you will need to define DOCKER_ENDPOINT " \
            "in your environment."
        errorExit "Script aborting"
    }

    set lines [split $lines \n]
    if {[llength $lines] != 1} {
        err err "There appear to be multiple Docker bridges on this system."
        err err "We don't know which one to use."
        err err "You will need to define DOCKER_ENDPOINT in your environment."
        errorExit "Script aborting"
    }

    if {![regexp {\d+\.\d+\.\d+\.\d+} [lindex $lines 0] ip]} {
        err err "We can't find the Docker IP address in the network configuration!"
        err err "Please ask someone to debug this script."
        err err \
            "Meanwhile, you will need to define DOCKER_ENDPOINT " \
            "in your environment."
        errorExit "Script aborting"
    }
        
    if {$i_tls} {
        set port 2376
    } else {
        set port 2375
    }

    if {![wait-for-it $ip:$port 0]} {
        err err "The Docker bridge $ip:$port does not seem to be working."
        err err "Make sure everything (including TLS) is configured correctly."
        err err \
            "If you are using non-standard Docker ports you will need " \
            "to define DOCKER_ENDPOINT."
        errorExit "Script aborting"
    }

    return http://$ip:$port
}


############################################################################
# ::busywork::peersFromSpec i_spec

# This procedure implements a standard way for busywork utilities to process
# specifications of subsets of peers.  The peers to target are named by the
# peer ids as they appear in the busywork 'network' file. Peers names can be
# given as a whitespace separated list or a comma-separated token.
# "Globbing-style" multi-peer abbreviations can also be used (implmented by
# the Tcl 'string match' command), but be aware that abbreviations must be
# entered in quotes to avoid globbing by the shell.  Examples of peer
# specifications:
# 
#    "*"           # Apply command to all peers
#    vp0           # Target vp0 only
#    vp0,vp1       # Target vp0 and vp1 only
#    "vp0 vp1"     # Same as above
#    "vp[01]"      # Also same as above
#    "vp[0-1]"     # Ditto
#    "vp0 vp1,vp2" # If you insist, this works too!
# 
# Note that to help gaurd against typos, the script will fail if there are any
# duplicate peer names in the peer specification, or if any peer specification
# does not match at least one peer, or if the peer specification is empty.
#
# The return value is the final list of peer names.

proc ::busywork::peersFromSpec {i_spec} {

    if {[null $i_spec]} {
        errorExit "No peers specified"
    }

    networkToArray network ""
    set validPeers $network(peer.ids)

    set specs {}
    foreach subSpec [split $i_spec ,] {
        set specs [concat $specs $subSpec]
    }

    set peers {}
    foreach spec $specs {
        foreach peer $validPeers {
            if {[string match $spec $peer]} {
                lappend peers $peer
            }
        }
    }

    set dups [duplicates $peers]
    if {![null $dups]} {
        errorExit "Duplicated peers: $dups"
    }

    set invalidPeers [setDifference $peers $validPeers]
    if {![null $invalidPeers]} {
        errorExit "Invalid peers: $invalidPeers"
    }

    return $peers
}


############################################################################
############################################################################
# ::busywork::Logger ?... args ...?

# The busywork::Logger object encapulates an asynchronous fabricLogger
# processes. The object starts the logger, and provides the calling
# environment simple services provides by the log, e.g., transaction ID (UUID)
# matching for interlock.

# Constructor arguments:
#
# -peers <peers> : Required
#
#     A list of fabric peer <host>:<port>. The <port> must be the REST API
#     port. For load balancing the logger will hit the peers in simple
#     round-robin order. Note that the peer network must be up and running,
#     and at least the genesis block must have been created.
#
# -file <path> : Defaults to $BUSYWORK_HOME/fabricLog
#
#     If specified, the log file is stored here instead of in the default
#     location. If <path> is specified as /tmp/, the log file is created with
#     'mktemp' .
#
# -keepLog | -noKeepLog : -noKeepLog
#
#     By default, the fabricLogger log file used for logging is deleted when the
#     calling context stops.  Use -keepLog to keep it from being deleted.
#
# -retry : 0
#
#     If > 0, then failing HTTP requests will be retried at most this many
#     times.
#
# -from <first>[:<last>] : All blocks, following from 0
#
#     This parameter can be used to restrict the scope of the logging. See the
#     fabricLogger utility for details, keeping in mind that the Logger object
#     always starts the associated fabricLogger in -follow mode.
#
# -idleWait <duration> : 10ms
#
#     There seems to be a bug somewhere, and this is a workaround for that
#     bug. It seems that when we are "following" the log file, we are seeing
#     the log file both readable from the context of Tcl fileevent, and also
#     showing EOF status. This causes the waitUUIDs routine to consume large
#     amounts of non-productive CPU time when transaction rates drop due to
#     consensus, because of continous "readable" events firing. The -idleWait
#     timer is used whenver we observe a read of the log file that does not
#     return at least 1 full line. After this failed read we wait -idleWait
#     before allowing the next "readable" event to fire.
#
# -timestamp | -noTimestamp : -noTimestamp
#
#     If -timestamp is specified, then error logs will be timestamped.
#
# -killOnError | -noKillOnError : -noKillOnError
#
#     If set as -killOnError, the process will be killed if the logger dies.
#
# -verbose <level> : 1
#
#     Level 0  : No messages of any kind
#     Level 1  : Print the name of the log file and its status
#     Level 2+ : ?

oo::class create ::busywork::Logger {

    # Option variables
    variable d_peers
    variable d_fabricLogger
    variable d_keepLog
    variable d_idleWait
    variable d_retry
    variable d_verbose
    variable d_timestamp
    
    # Implementation variables
    variable d_logFile
    variable d_logChannel
    variable d_gets
    
    constructor {args} {

        set options {
            {key:req -peers                        d_peers}
            {key     -file                         file       {}  p_file}
            {bool    {-keepLog -noKeepLog}         d_keepLog   0}
            {key     -retry                        d_retry     0}
            {key     -from                         from        {} p_from}
            {key     -idlewait                     d_idleWait  10ms}
            {bool    {-timestamp -noTimestamp}     d_timestamp 0}
            {bool    {-killOnError -noKillOnError} killOnError 0}
            {key     -verbose                      d_verbose   1}
        }

        mapKeywordArgs $args $options

        if {$p_file} {
            if {$file eq "/tmp/"} {
                set d_logFile [exec mktemp -t fabricLog.XXXXX]
            } else {
                set d_logFile $file
            }
        } else {
            set d_logFile [busywork::home]/fabricLog
        }
        exec touch $d_logFile
        set d_idleWait [durationToMs $d_idleWait]

        set command "[busywork::bin]/fabricLogger -file $d_logFile"
        set command "$command -follow -followPoll 10ms -retry $d_retry"
        if {$d_timestamp} {
            set command "$command -timestamp"
        }
        if {!$d_keepLog} {
            set command "$command -delete"
        }
        if {$killOnError} {
            set command "$command -killOnError [pid]"
        }
        if {$p_from} {
            set command "$command -from $from"
        }
        set command [concat $command $d_peers]

        set pid [eval exec $command &]
        killAtExit SIGINT $pid

        if {[waitFor 10s {expr {[file size $d_logFile] > 0}}]} {
            errorExit "Wait for fabricLogger to start timed out"
        }

        my Open

        if {$d_verbose >= 1} {
            note note "busywork::Logger : Logging to $d_logFile"
            if {$d_keepLog} {
                note note "The log file will persist after exit"
            } else {
                note note "The log file will be deleted at exit"
            }
        }
    }

    destructor {

        my Close
    }
    

    # Private methods
    
    # Open the log file and create the NonblockingGets object. See the
    # documentation of NonblockingGets for why this is necessary.
    method Open {} {

        set d_logChannel [open $d_logFile r]
        fconfigure $d_logChannel -blocking 0 -buffering line
        set d_gets [NonblockingGets new $d_logChannel]
    }


    # Close the log file and destroy the NonblockingGets object.
    method Close {} {

        close $d_logChannel
        $d_gets destroy
    }
}

# reset
#
# The reset{} method closes and reopens the log file. This is to avoid
# confusion for example in fork()-ed child processes.

oo::define ::busywork::Logger {

    method reset {} {

        my Close
        my Open
    }
}


# waitUUIDs i_type i_uuids {i_timeout -1} {progress 0}
#
# Wait until the fabricLogger-generated stream has recorded a set of UUIDs in
# the blockchain. In this general implementation no assumption on the order of
# the UUIDs is made. However it is assumed that none of the UUIDs have been
# searched in the channel, and also assumed that non-matching UUIDs can be
# safely discarded, i.e., they will never need to be matched in the future on
# this stream.
#
# i_type is one of
#
#     deploy | invoke
#
# i_uuids is a list of UUIDs. For deploy transactions these "UUID" are
# currently required to be the chaincode name. The i_timeout is optional, and
# is specified as a duration (see durationToMs). If the timeout is < 0 then
# the routine will wait forever until all UUID have been seen. If the timeout
# is >= 0 then if not all UUID have been seen before the timeout, a list of
# the unseen UUID will be returned in an arbitrary order. The successful
# return value is always the empty list.
#
# NB: Very short timeouts will likely always fail due to the way the timeout
# is implemented.
#
# The implementation is straightforward - We simply create an array of the
# UUID names and mark them off as they are seen.

oo::define ::busywork::Logger {

    variable d_code
    variable d_unseen
    variable d_count
    variable d_status

    method waitUUIDs {i_type i_uuids {i_timeout -1}} {

        if {[null $i_uuids]} return

        set timeout [durationToMs $i_timeout]
        switch $i_type {
            deploy {set d_code d}
            invoke {set d_code i}
            default {
                errorx \
                    "busywork::Logger::waitUUIDs : " \
                    "Illegal value for i_type -> '$i_type'"
            }
        }

        array unset d_unseen
        foreach uuid $i_uuids {
            set d_unseen($uuid) {}
        }
        set d_count [llength [array names d_unseen]]
    
        set d_status {}
        set timer {}
        if {$timeout >= 0} {
            set timer [after $timeout [list [self] waitUUIDsTimeout]]
        }
        fileevent $d_logChannel readable [list [self] waitUUIDsProcess]
        vwait [self namespace]::d_status
        after cancel $timer

        return [array names d_unseen]
    }


    # Timeout the UUID wait
    method waitUUIDsTimeout {} {

        fileevent $d_logChannel readable {}
        set d_status timeout
    }


    # Process the readable log file
    method waitUUIDsProcess {} {

        set got 0
        while {1} {

            if {![$d_gets gets]} {
                if {!$got} {
                    after $d_idleWait
                }
                break
            }
            set got 1
            set line [$d_gets line]

            if {[lindex $line 0] eq $d_code} {
                set uuid [lindex $line 1]
                if {[info exists d_unseen($uuid)]} {
                    array unset d_unseen $uuid
                    if {[incr d_count -1] == 0} {
                        fileevent $d_logChannel readable {}
                        set d_status done
                        break
                    }
                }
            }
        }
    }
}
