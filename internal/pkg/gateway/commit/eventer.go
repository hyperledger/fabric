/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import "context"

type Eventer struct {
	Notifier *Notifier
}

func (e *Eventer) ChaincodeEvents(ctx context.Context, channelName string, chaincodeName string) (<-chan *BlockChaincodeEvents, error) {
	return e.Notifier.notifyChaincodeEvents(ctx.Done(), channelName, chaincodeName)
}
