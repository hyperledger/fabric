# fabric.tcl - A Tcl support package for Hyperledger fabric scripts

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

# Note: The logging level is controlled by the 'fabric' module.

package require http
package require json
package require utils
package require yaml

package provide fabric 0.0
namespace eval ::fabric {}

############################################################################
# chaincode i_peer i_method i_query {i_retry 0}

# Make a REST API 'devops' query. The i_peer is the full host:port
# address. The i_method must be 'deploy', 'invoke' or 'query'.

# If 'i_retry' is greater than 0, then HTTP failures are logged but retried up
# to that many times. Incorrectly formatted data returned from a valid query
# is never retried. We currently do not implememnt retry backoffs - it's
# pedal-to-the-metal.

# If i_retry is less than 0 on entry, then this is a special flag that the
# caller is expecting HTTP failures, and does not want diagnostics or error
# exits. If the HTTP access fails then the call will exit with Tcl error{} and
# the caller will presumably catch{} the error and do whatever is appropriate.

proc ::fabric::chaincode {i_peer i_method i_query {i_retry 0}} {

    for {set retry [math:::max $i_retry 0]} {$retry >= 0} {incr retry -1} {

        if {[catch {
            ::http::geturl http://$i_peer/chaincode -query $i_query
        } token]} {
            if {$i_retry < 0} {
                http::cleanup $token
                error "http::geturl failed"
            }
            if {$retry > 0} {
                if {$retry == $i_retry} {
                    warn fabric \
                        "fabric::chaincode $i_peer $i_method: " \
                        "Retrying after catastrophic HTTP error"
                }
                http::cleanup $token
                continue
            }
            if {($retry == 0) && ($i_retry != 0)} {
                err fabric \
                    "fabric::chaincode $i_peer $i_method: " \
                    "Retry limit ($i_retry) hit after " \
                    "catastrophic HTTP error : Aborting"
            }
            http::cleanup $token
            errorExit \
                "fabric::chaincode $i_peer $i_method: ::http::geturl failed\n" \
                $::errorInfo
        }

        if {[http::ncode $token] != 200} {
            if {$i_retry < 0} {
                http::cleanup $token
                error "http::ncode != 200"
            }
            if {$retry > 0} {
                if {$retry == $i_retry} {
                    warn fabric \
                        "fabric::chaincode $i_peer $i_method: " \
                        "Retrying after HTTP error return"
                }
                if {($retry == 0) && ($i_retry != 0)} {
                    err fabric \
                        "fabric::chaincode $i_peer $i_method: " \
                        "Retry limit ($i_retry) hit after " \
                        "HTTP error return : Aborting"
                }
                http::cleanup $token
                continue
            }
            err fabric \
                "fabric::chaincode '$i_method' transaction to $i_peer failed " \
                "with ncode = '[http::ncode $token]'; Aborting\n"
            httpErrorExit $token
        }

        set response [http::data $token]

        set fail [catch {
            set parse [json::json2dict $response]
            set result [dict get $parse result]
            set status [dict get $result status]
            if {$status ne "OK"} {
                error "Status not OK"
            }
            set message [dict get $result message]
        } err]

        if {$fail} {

            set msg \
                [concat \
                     "fabric::chaincode '$i_method' response from $i_peer " \
                     "is malformed/unexpected: $err"]

            if {$i_retry < 0} {
                http::cleanup $token
                error $msg

            } else {

                err fabric $msg
                httpErrorExit $token
            }
        }

        http::cleanup $token

        if {($i_retry >= 0) && ($retry != $i_retry)} {
            note fabric \
                "fabric::chaincode $i_peer $i_method: " \
                "Success after [expr {$i_retry - $retry}] HTTP retries"
        }
        break
    }

    return $message
}


############################################################################
# deploy i_peer i_user i_chaincode i_fn i_args {i_retry 0}

# Deploy a GOLANG chaincode to the network. The i_peer is the full network
# address (host:port) of the REST API port of the peer.  If i_user is
# non-empty then this will be a secure transaction. The constructor will apply
# i_fn to i_args.

# See ::fabric::chaincode{} for a discussion of the 'i_retry' parameter.

proc ::fabric::deploy {i_peer i_user i_chaincode i_fn i_args {i_retry 0}} {

    set template {
        "jsonrpc" : "2.0",
        "method" : "deploy",
        "params" : {
            "type": 1,
            "chaincodeID" : {
                "path" : "$i_chaincode"
            },
            "ctorMsg" : {
                "args" : [$args]
            },
            "secureContext": "$i_user"
        },
        "id": 1
    }

    set args [argify $i_fn $i_args]
    set query [list [subst -nocommand $template]]

    return [chaincode $i_peer deploy $query $i_retry]
}


############################################################################
# devModeDeploy i_peer i_user i_chaincode i_fn i_args {i_retry 0}

# Issue a "deploy" transaction for a pre-started chaincode in development
# mode. Here, the i_chaincode is a user-specified name. All of the other
# arguments are otherwise the same as for deploy{}.

# See ::fabric::chaincode{} for a discussion of the 'i_retry' parameter.

proc ::fabric::devModeDeploy {i_peer i_user i_chaincode i_fn i_args {i_retry 0}} {

    set template {
        "jsonrpc" : "2.0",
        "method" : "deploy",
        "params" : {
            "type": 1,
            "chaincodeID" : {
                "name" : "$i_chaincode"
            },
            "ctorMsg" : {
                "args" : [$args]
            },
            "secureContext": "$i_user"
        },
        "id": 1
    }

    set args [argify $i_fn $i_args]
    set query [list [subst -nocommand $template]]

    return [devops $i_peer deploy $query $i_retry]
}

############################################################################
# invoke i_peer i_user i_chaincode i_fn i_args {i_retry 0}

# Invoke a GOLANG chaincode on the network. The i_peer is the full network
# address (host:port) of the REST API port of the peer. If i_user is non-empty
# then this will be a secure transaction. The i_chaincodeName is the hash used
# to identify the chaincode. The invocation will apply i_fn to i_args.

# See ::fabric::chaincode{} for a discussion of the 'i_retry' parameter.

proc ::fabric::invoke {i_peer i_user i_chaincodeName i_fn i_args {i_retry 0}} {

    set template {
        "jsonrpc" : "2.0",
        "method" : "invoke",
        "params" : {
            "type": 1,
            "chaincodeID" : {
                "name" : "$i_chaincodeName"
            },
            "ctorMsg" : {
                "args" : [$args]
            },
            "secureContext": "$i_user"
        },
        "id": 1
    }

    set args [argify $i_fn $i_args]
    set query [list [subst -nocommand $template]]

    return [chaincode $i_peer invoke $query $i_retry]
}


############################################################################
# query i_peer i_user i_chaincodeName i_fn i_args {i_retry 0}

# Query a GOLANG chaincode on the network. The i_peer is the full network
# address (host:port) of the REST API port of the peer. If i_user is non-empty
# then this will be a secure transaction. The i_chaincodeName is the hash used
# to identify the chaincode. The query will apply i_fn to i_args.

# See ::fabric::chaincode{} for a discussion of the 'i_retry' parameter.

proc ::fabric::query {i_peer i_user i_chaincodeName i_fn i_args {i_retry 0}} {

    set template {
        "jsonrpc" : "2.0",
        "method" : "query",
        "params" : {
            "type": 1,
            "chaincodeID" : {
                "name" : "$i_chaincodeName"
            },
            "ctorMsg" : {
                "args" : [$args]
            },
            "secureContext": "$i_user"
        },
        "id": 1
    }

    set args [argify $i_fn $i_args]
    set query [list [subst -nocommand $template]]

    return [chaincode $i_peer query $query $i_retry]
}


############################################################################
# height i_peer {i_retry 0}

# Call the REST /chain API, returning the block height

proc ::fabric::height {i_peer {i_retry 0}} {

    for {set retry $i_retry} {$retry >= 0} {incr retry -1} {

        if {[catch {http::geturl http://$i_peer/chain} token]} {
            if {$retry > 0} {
                if {$retry == $i_retry} {
                    warn fabric \
                        "$i_peer /chain: Retrying after catastrophic HTTP error"
                }
                http::cleanup $token
                continue
            }
            errorExit \
                "$i_peer /chain: ::http::geturl failed " \
                "with $i_retry retries : $token"
        }

        if {[http::ncode $token] != 200} {

            # Failure

            if {$retry > 0} {
                if {$retry == $i_retry} {
                    warn fabric \
                        "$i_peer /chain: Retrying after HTTP error return"
                }
                http::cleanup $token
                continue
            }

            err fabric \
                "$i_peer /chain; REST API call failed with $i_retry retries"
            httpErrorExit $token
        }

        if {[catch {json::json2dict [http::data $token]} parse]} {
            err fabric "$i_peer /chain: JSON response does not parse: $parse"
            httpErrorExit $token
        }

        if {[catch {dict get $parse height} height]} {
            err fabric \
                "$i_peer /chain: HTTP response does not contain a height: " \
                $height
            httpErrorExit $token
        }

        return $height
    }
}


############################################################################
# checkForLocalDockerChaincodes i_nPeers i_chaincodeNames

# Given a system with i_nPeers peers, e.g., a Docker compose setup, return 1 if
# we see i_nPeers copies of each chaincode name in the Docker 'ps' listing of
# running containers, else 0.

proc ::fabric::checkForLocalDockerChaincodes {i_nPeers i_chaincodeNames} {

    foreach name $i_chaincodeNames {

        catch [list exec docker ps -f status=running | grep -c $name] count
        if {$count != $i_nPeers} {return 0}
    }
    return 1
}


############################################################################
# dockerLocalPeerContainers
# dockerLocalPeerIPs

# Return a list of docker container IDs for all running containers based on
# the 'hyperledger/fabric-peer' image.

proc ::fabric::dockerLocalPeerContainers {} {

    # This does not seem to work; membersrvc may (appear to?) be built from
    # the hyperledger/fabric-peer image.
    #return [exec docker ps -q -f status=running -f ancestor=hyperledger/fabric-peer]

    return [exec docker ps --format="{{.Image}}\ {{.ID}}" | \
                grep ^hyperledger/fabric-peer | cut -f 2 -d " " ]
}


# Return a list of IP addresses of running containers based on the
# 'hyperledger/fabric-peer' image. The IP addresses are returned in sorted order.

proc ::fabric::dockerLocalPeerIPs {} {

    return [lsort \
                [mapeach container [dockerLocalPeerContainers] {
                    exec docker inspect \
                        --format {{{.NetworkSettings.IPAddress}}} $container
                }]]
}

# Return a list of pairs of {<name> <IP>} for running containers based on the
# 'hyperledger/fabric-peer' image. The pairs are returned sorted on the container
# name.

proc ::fabric::dockerLocalPeerNamesAndIPs {} {

    set l \
        [mapeach container [dockerLocalPeerContainers] {
            list \
                 [exec docker inspect --format {{{.Name}}} $container] \
                 [exec docker inspect \
                      --format {{{.NetworkSettings.IPAddress}}} $container]
        }]
    return [lsort -dictionary -index 0 $l]
}


############################################################################
# Security
############################################################################

############################################################################
# caLogin user secret

# Log in a user

proc ::fabric::caLogin {i_peer i_user i_secret} {

    set template {
        {
            "enrollId"     : "$i_user",
            "enrollSecret" : "$i_secret"
        }
    }

    set query [subst $template]

    set token [::http::geturl http://$i_peer/registrar -query $query]
    if {[http::ncode $token] != 200} {
        errorExit "fabric::caLogin : Login failed for user $i_user on peer $i_peer"
    }
    http::cleanup $token
}


############################################################################
# argify i_fn i_args

# Convert old-style fn + args pair into a list of quoted arguments with
# commas to satisfy the most recent JSON format of the REST API. This needs to
# be done as a string (rather than as a list), otherwise it will be {} quoted
# when substituted.

proc ::fabric::argify {i_fn i_args} {

    set args [concat $i_fn $i_args]
    set jsonargs ""
    set comma ""
    foreach arg $args {
        set jsonargs "$jsonargs$comma\"$arg\""
        set comma ,
    }
    return $jsonargs
}
