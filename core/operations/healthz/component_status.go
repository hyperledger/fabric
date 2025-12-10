/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthz

import (
	"time"
)

type ComponentStatus struct {
	Status  string                 `json:"status"`  // "OK", "DEGRADED", or "UNAVAILABLE"
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// DetailedHealthStatus is used by the /healthz/detailed endpoint.
type DetailedHealthStatus struct {
	Status       string                        `json:"status"` // "OK", "DEGRADED", or "UNAVAILABLE"
	Time         time.Time                     `json:"time"`
	Components   map[string]ComponentStatus    `json:"components"`
	FailedChecks []FailedCheck                 `json:"failed_checks,omitempty"` // for backward compatibility
}

const (
	StatusOK          = "OK"
	StatusDegraded    = "DEGRADED"
	StatusUnavailable = "UNAVAILABLE"
)

// FailedCheck is compatible with fabric-lib-go.
type FailedCheck struct {
	Component string `json:"component"`
	Reason    string `json:"reason"`
}

