/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/peer"
)

type addr struct{}

func (*addr) Network() string {
	return ""
}

func (*addr) String() string {
	return "1.2.3.4:5000"
}

func TestExtractAddress(t *testing.T) {
	ctx := context.Background()
	require.Zero(t, ExtractRemoteAddress(ctx))

	ctx = peer.NewContext(ctx, &peer.Peer{
		Addr: &addr{},
	})
	require.Equal(t, "1.2.3.4:5000", ExtractRemoteAddress(ctx))
}
