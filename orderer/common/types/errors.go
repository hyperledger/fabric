/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package types

import "github.com/pkg/errors"

// ErrSystemChannelExists is returned when trying to join or remove an application channel when the system channel exists.
//
// Deprecated: system channel no longer supported
var ErrSystemChannelExists = errors.New("system channel exists")

// ErrSystemChannelNotSupported  is returned when trying to join with a system channel config block.
var ErrSystemChannelNotSupported = errors.New("system channel not supported")

// ErrChannelAlreadyExists is returned when trying to join a app channel that already exists (when the system channel does not
// exist), or when trying to join the system channel when it already exists.
var ErrChannelAlreadyExists = errors.New("channel already exists")

// ErrAppChannelsAlreadyExists is returned when trying to join a system channel (that does not exist) when application channels
// already exist.
var ErrAppChannelsAlreadyExists = errors.New("application channels already exist")

// ErrChannelNotExist is returned when trying to remove or list a channel that does not exist
var ErrChannelNotExist = errors.New("channel does not exist")

// ErrChannelPendingRemoval is returned when trying to remove or list a channel that is being removed.
var ErrChannelPendingRemoval = errors.New("channel pending removal")

// ErrChannelRemovalFailure is returned when a removal attempt failure has been recorded.
var ErrChannelRemovalFailure = errors.New("channel removal failure")
