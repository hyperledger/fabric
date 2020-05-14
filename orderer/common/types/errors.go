/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package types

import "github.com/pkg/errors"

// This error is returned when trying to join or remove an application channel when the system channel exists.
var ErrSystemChannelExists = errors.New("system channel exists")

// This error is returned when trying to create a channel that already exists.
var ErrChannelAlreadyExists = errors.New("channel already exists")

// This error is returned when trying to create a system channel when application channels already exist.
var ErrAppChannelsAlreadyExists = errors.New("application channels already exist")
