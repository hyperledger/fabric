#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
#
# usage: ./pte_driver.sh <user input file>
# example: ./pte_driver.sh runCases.txt
#
#    runCases.txt:
#    node userInputs/userInput-samplecc-i.json
#    node userInputs/userInput-samplecc-q.json
#

inFile=$1
EXENODE=pte-main.js
nNetwork=0

while read line
do
   #echo $line
   tt=$(echo $line | awk '{print $1}')
   #echo " tt  $tt"
   sdkType=$(echo $tt | awk '{print tolower($tt)}')
   #echo "tt $tt sdkType $sdkType"
   userinput=$(echo $line | awk '{print $2}')

   case $sdkType in
     sdk=node)
       echo "sdk type spported: $sdkType"
       nodeArray[${#nodeArray[@]}]=$userinput
       ;;

     sdk=python)
       echo "sdk type unspported: $sdkType"
       pythonArray[${#pythonArray[@]}]=$userinput
       ;;

     sdk=java)
       echo "sdk type unspported: $sdkType"
       javaArray[${#javaArray[@]}]=$userinput
       ;;

     *)
       echo "sdk type unknown: $sdkType"
       ;;

   esac

done < $1

echo "Node Array: ${nodeArray[@]}"

# node requests
function nodeProc {
    nNetwork=${#nodeArray[@]}
    tWait=$[nNetwork*4000+10000]
    tCurr=`date +%s%N | cut -b1-13`
    tStart=$[tCurr+tWait]
    echo "nNetwork: $nNetwork, tStart: $tStart"

    BCN=0
    for i in ${nodeArray[@]}; do
        echo "execution: $i"
        node $EXENODE $BCN $i $tStart &
        let BCN+=1
    done
}

# node requests
function pythonProc {
    echo "python has not supported yet."
}

# node requests
function javaProc {
    echo "java has not supported yet."
}

# node
if [ ${#nodeArray[@]} -gt 0 ]; then
    echo "executing ${#nodeArray[@]} node requests"
    nodeProc
else
    echo "no node requests"
fi

# python
if [ ${#pythonArray[@]} -gt 0 ]; then
    echo "executing ${#pythonArray[@]} python requests"
    pythonProc
else
    echo "no python requests"
fi

# java
if [ ${#javaArray[@]} -gt 0 ]; then
    echo "executing ${#javaArray[@]} java requests"
    javaProc
else
    echo "no java requests"
fi

exit
