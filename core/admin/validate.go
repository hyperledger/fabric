/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package admin

import (
	"context"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var (
	accessDenied = errors.New("access denied")
	timeDiff     = time.Minute * 15
)

type validator struct {
	ace AccessControlEvaluator
}

func (v *validator) validate(ctx context.Context, env *common.Envelope) (*peer.AdminOperation, error) {
	op, sd, err := validateStructure(ctx, env)
	if err != nil {
		return nil, err
	}
	addr := util.ExtractRemoteAddress(ctx)
	if err := v.ace.Evaluate(sd); err != nil {
		logger.Warningf("Request from %s unauthorized due to authentication: %v", addr, err)
		return nil, accessDenied
	}

	return op, nil
}

func validateStructure(ctx context.Context, env *common.Envelope) (*peer.AdminOperation, []*common.SignedData, error) {
	if ctx == nil {
		return nil, nil, errors.New("nil context")
	}
	if env == nil {
		return nil, nil, errors.New("nil envelope")
	}
	addr := util.ExtractRemoteAddress(ctx)
	op := &peer.AdminOperation{}
	ch, err := utils.UnmarshalEnvelopeOfType(env, common.HeaderType_PEER_ADMIN_OPERATION, op)
	if err != nil {
		logger.Warningf("Request from %s is badly formed: +%v", addr, err)
		return nil, nil, errors.Wrap(err, "bad request")
	}

	if ch.Timestamp == nil {
		logger.Warningf("Request from %s has no timestamp", addr)
		return nil, nil, errors.Errorf("empty timestamp")
	}
	ts := ch.Timestamp
	reqTs := time.Unix(ts.Seconds, int64(ts.Nanos))
	now := time.Now()
	if reqTs.Add(timeDiff).Before(now) || reqTs.Add(-timeDiff).After(now) {
		logger.Warningf("Request from %s unauthorized due to incorrect time: %s", addr, reqTs.String())
		return nil, nil, accessDenied
	}
	sd, err := env.AsSignedData()
	if err != nil {
		return nil, nil, errors.Errorf("bad request, cannot extract signed data: %v", err)
	}
	return op, sd, nil
}
