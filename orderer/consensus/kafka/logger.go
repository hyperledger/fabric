/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/common/flogging"
	logging "github.com/op/go-logging"
)

const (
	pkgLogID    = "orderer/consensus/kafka"
	saramaLogID = pkgLogID + "/sarama"
)

var logger *logging.Logger

// init initializes the package logger
func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

// init initializes the samara logger
func init() {
	loggingProvider := flogging.MustGetLogger(saramaLogID)
	loggingProvider.ExtraCalldepth = 3
	sarama.Logger = &saramaLoggerImpl{
		logger: loggingProvider,
	}
}

type saramaLoggerImpl struct {
	logger *logging.Logger
}

func (l saramaLoggerImpl) Print(args ...interface{}) {
	l.print(fmt.Sprint(args...))
}

func (l saramaLoggerImpl) Printf(format string, args ...interface{}) {
	l.print(fmt.Sprintf(format, args...))
}

func (l saramaLoggerImpl) Println(args ...interface{}) {
	l.print(fmt.Sprintln(args...))
}

func (l saramaLoggerImpl) print(message string) {
	l.logger.Debug(message)
}
