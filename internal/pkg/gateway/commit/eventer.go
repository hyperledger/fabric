/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import "context"

type Eventer struct {
	notifier *Notifier
}

func NewEventer(notifier *Notifier) *Eventer {
	return &Eventer{
		notifier: notifier,
	}
}

func (e *Eventer) ChaincodeEvents(ctx context.Context, channelName string, chaincodeName string) (<-chan *BlockChaincodeEvents, error) {
	return e.notifier.notifyChaincodeEvents(ctx.Done(), channelName, chaincodeName)
}
