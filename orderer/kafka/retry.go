/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"fmt"
	"time"

	localconfig "github.com/hyperledger/fabric/orderer/localconfig"
)

type retryProcess struct {
	shortPollingInterval, shortTimeout time.Duration
	longPollingInterval, longTimeout   time.Duration
	exit                               chan struct{}
	channel                            channel
	msg                                string
	fn                                 func() error
}

func newRetryProcess(retryOptions localconfig.Retry, exit chan struct{}, channel channel, msg string, fn func() error) *retryProcess {
	return &retryProcess{
		shortPollingInterval: retryOptions.ShortInterval,
		shortTimeout:         retryOptions.ShortTotal,
		longPollingInterval:  retryOptions.LongInterval,
		longTimeout:          retryOptions.LongTotal,
		exit:                 exit,
		channel:              channel,
		msg:                  msg,
		fn:                   fn,
	}
}

func (rp *retryProcess) retry() error {
	if err := rp.try(rp.shortPollingInterval, rp.shortTimeout); err != nil {
		logger.Debugf("[channel: %s] Switching to the long retry interval", rp.channel.topic())
		return rp.try(rp.longPollingInterval, rp.longTimeout)
	}
	return nil
}

func (rp *retryProcess) try(interval, total time.Duration) error {
	// Configuration validation will not allow non-positive ticker values
	// (which would result in panic). The path below is for those test cases
	// when we cannot avoid the creation of a retriable process but we wish
	// to terminate it right away.
	if rp.shortPollingInterval == 0*time.Second {
		return fmt.Errorf("illegal value")
	}

	var err error

	// If initial operation is successful, we don't bother start retry process
	logger.Debugf("[channel: %s] "+rp.msg, rp.channel.topic())
	if err := rp.fn(); err == nil {
		logger.Debugf("[channel: %s] Error is nil, breaking the retry loop", rp.channel.topic())
		return err
	}

	tickInterval := time.NewTicker(interval)
	tickTotal := time.NewTicker(total)
	defer tickTotal.Stop()
	defer tickInterval.Stop()
	logger.Debugf("[channel: %s] Retrying every %s for a total of %s", rp.channel.topic(), interval.String(), total.String())

	for {
		select {
		case <-rp.exit:
			exitErr := fmt.Errorf("[channel: %s] process asked to exit", rp.channel.topic())
			logger.Warning(exitErr.Error()) // Log it at the warning level
			return exitErr
		case <-tickTotal.C:
			return err
		case <-tickInterval.C:
			logger.Debugf("[channel: %s] "+rp.msg, rp.channel.topic())
			if err = rp.fn(); err == nil {
				logger.Debugf("[channel: %s] Error is nil, breaking the retry loop", rp.channel.topic())
				return err
			}
		}
	}
}
