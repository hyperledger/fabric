/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package main        // Orderer Test Engine

import (
        "fmt"
        "testing"
        "time"
        "strconv"
)

// simplest testcase
func Test_1tx_1ch_1ord_Solo(t *testing.T) {
        fmt.Println("\nSimplest test: Send 1 TX on 1 channel to 1 Solo orderer")
        passResult, finalResultSummaryString := ote("Test_1tx_1ch_1ord_Solo", 1, 1, 1, "solo", 0, false, 1 )
        t.Log(finalResultSummaryString)
        if !passResult { t.Fail() }
}

// 76 - moved below

// 77, 78 = rerun with batchsize = 500 // CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT=500
func Test_ORD77_ORD78_10000TX_1ch_1ord_solo_batchSz(t *testing.T) {
        //fmt.Println("Send 10,000 TX on 1 channel to 1 Solo orderer")
        passResult, finalResultSummaryString := ote("ORD-77_ORD-78", 10000, 1, 1, "solo", 0, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 79, 80 = rerun with batchsize = 500
func Test_ORD79_ORD80_10000TX_1ch_1ord_kafka_1kbs_batchSz(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-79,ORD-80", 10000, 1, 1, "kafka", 1, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 81, 82 = rerun with batchsize = 500
func Test_ORD81_ORD82_10000TX_3ch_1ord_kafka_3kbs_batchSz(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-81,ORD-82", 10000, 3, 1, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 83, 84 = rerun with batchsize = 500
func Test_ORD83_ORD84_10000TX_3ch_3ord_kafka_3kbs_batchSz(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-83,ORD-84", 10000, 3, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 85
func Test_ORD85_1000000TX_1ch_3ord_kafka_3kbs_spy(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-85", 1000000, 1, 3, "kafka", 3, true, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 86
func Test_ORD86_1000000TX_3ch_1ord_kafka_3kbs_spy(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-86", 1000000, 1, 1, "kafka", 3, true, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 87
func Test_ORD87_1000000TX_3ch_3ord_kafka_3kbs_spy(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-87", 1000000, 3, 3, "kafka", 3, true, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

/* The "3 producers" option is not supported, so no need to run these tests yet
// 88
func Test_ORD88_1000000TX_1ch_1ord_kafka_3kbs_spy_3ppc(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-88", 1000000, 1, 1, "kafka", 3, true, 3 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 89
func Test_ORD89_1000000TX_3ch_3ord_kafka_3kbs_spy_3ppc(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-89", 1000000, 3, 3, "kafka", 3, true, 3 )
        if !passResult { t.Error(finalResultSummaryString) }
}
*/

// 90
func Test_ORD90_1000000TX_100ch_1ord_kafka_3kbs_spy(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-90", 1000000, 100, 1, "kafka", 3, true, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 91
func Test_ORD91_1000000TX_100ch_3ord_kafka_3kbs(t *testing.T) {
        passResult, finalResultSummaryString := ote("ORD-91", 1000000, 100, 3, "kafka", 3, true, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

func pauseAndUnpause(target string) {
        time.Sleep(40 * time.Second)
        executeCmd("docker pause " + target + "; sleep 30; docker unpause " + target + " ")
}

func stopAndStart(target string) {
        time.Sleep(40 * time.Second)
        executeCmd("docker stop " + target + "; sleep 30; docker start " + target)
}

func stopAndStartAllTargetOneAtATime(target string, num int) {
        fmt.Println("Stop and Start ", num, " " + target + "s sequentially")
        for i := 0 ; i < num ; i++ {
                stopAndStart(target + strconv.Itoa(i))
        }
        // A restart (below) is similar, but lacks delays in between
        // executeCmd("docker restart kafka0 kafka1 kafka2")
        fmt.Println("All ", num, " requested " + target + "s are stopped and started")
}

func pauseAndUnpauseAllTargetOneAtATime(target string, num int) {
        fmt.Println("Pause and Unpause ", num, " " + target + "s sequentially")
        for i := 0 ; i < num ; i++ {
                pauseAndUnpause(target + strconv.Itoa(i))
        }
        fmt.Println("All ", num, " requested " + target + "s are paused and unpaused")
}

// 76
func Test_ORD76_40000TX_1ch_1ord_kafka_3kbs(t *testing.T) {
        go stopAndStart("kafka0")
        passResult, finalResultSummaryString := ote("ORD-76", 100000, 1, 1, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

// 92 and 93 - orderer tests, moved below

//94 - stopAndStartAll KafkaBrokers OneAtATime
func Test_ORD94_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        go stopAndStartAllTargetOneAtATime("kafka", 3)
        passResult, finalResultSummaryString := ote("ORD-94", 500000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

//95 - pauseAndUnpauseAll KafkaBrokers OneAtATime
func Test_ORD95_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        go pauseAndUnpauseAllTargetOneAtATime("kafka", 3)
        passResult, finalResultSummaryString := ote("ORD-95", 500000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

//96 - stopping n-1 KBs
func Test_ORD96_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        go kafka_3kb_restart_2kb_delay()
        passResult, finalResultSummaryString := ote("ORD-96", 500000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}
func kafka_3kb_restart_2kb_delay() {
        time.Sleep(40 * time.Second)
        fmt.Println("Stopping 2 of 3 Kafka brokers")
        executeCmd("docker stop kafka0 ; sleep 30 ; docker stop kafka1 ; sleep 30 ; docker start kafka1 ; docker start kafka0")
        fmt.Println("kafka brokers are restarted")
}

//97 - stopping all the kafka brokers at once
func Test_ORD97_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        go kafka_3kb_restart_3kb()
        passResult, finalResultSummaryString := ote("ORD-96", 500000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}
func kafka_3kb_restart_3kb() {
        time.Sleep(40 * time.Second)
        fmt.Println("Stopping the Kafka brokers")
        executeCmd("docker stop kafka0 kafka1 kafka2 ; sleep 30 ; docker start kafka0 kafka1 kafka2")
        fmt.Println("All the kafka brokers are restarted")
}

//98 - pausing n-1 KBs
func Test_ORD98_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        go kafka_3kb_pause_2kb_delay()
        passResult, finalResultSummaryString := ote("ORD-97", 500000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}
func kafka_3kb_pause_2kb_delay() {
        time.Sleep(40 * time.Second)
        fmt.Println("Pausing the Kafka brokers")
        executeCmd("docker pause kafka0 ; sleep 30 ; docker pause kafka1 ; sleep 30 ; docker unpause kafka1 ; docker unpause kafka0")
        fmt.Println("All the kafka brokers are restarted")
}

//99 pausing all the kafka brokers at once
func Test_ORD99_500000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
       go kafka_3kb_pause_3kb()
       passResult, finalResultSummaryString := ote("ORD-97", 500000, 1, 3, "kafka", 3, false, 1 )
       if !passResult { t.Error(finalResultSummaryString) }
}
func kafka_3kb_pause_3kb() {
        time.Sleep(40 * time.Second)
        fmt.Println("Pausing the Kafka brokers")
        executeCmd("docker pause kafka0 kafka1 kafka2; sleep 30 ; docker unpause kafka0 kafka1 kafka2")
        fmt.Println("All the kafka brokers are restarted")
}

// For tests 92 and 93:
// As code works now, traffic will be dropped when orderer stops, so
// the number of transactions and blocks DELIVERED to consumers watching that
// orderer will be lower. So the OTE testcase will fail - BUT
// we could manually verify the ACK'd TXs match the delivered.

//92 stop an orderer (not orderer0, so we can still see progress logs)
func Test_ORD92_100000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        //go executeCmd("sleep 40; docker stop orderer1")
        go stopAndStart("orderer1")
        passResult, finalResultSummaryString := ote("ORD-92", 60000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

//93 pause an orderer (not orderer0, so we can still see progress logs)
func Test_ORD93_100000TX_1ch_3ord_kafka_3kbs(t *testing.T) {
        //go executeCmd("sleep 40; docker pause orderer1")
        go pauseAndUnpause("orderer1")
        passResult, finalResultSummaryString := ote("ORD-93", 60000, 1, 3, "kafka", 3, false, 1 )
        if !passResult { t.Error(finalResultSummaryString) }
}

