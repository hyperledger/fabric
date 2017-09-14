/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
)

func TestLoggerInit(t *testing.T) {
	assert.IsType(t, &saramaLoggerImpl{}, sarama.Logger, "Sarama logger not properly initialized")
}
