package bdls

import (
	"testing"
	"fmt"
	"net"
	"time"
	
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigUpdate(t *testing.T) {
	tests := []struct {
		name           string
		oldConsenters  []*common.Consenter
		newConsenters  []*common.Consenter
		expectAdded    int
		expectRemoved  int
		expectError    bool
	}{
		{
			name: "Add one node",
			oldConsenters: []*common.Consenter{
				{Host: "host1", Port: 7050},
				{Host: "host2", Port: 7050},
			},
			newConsenters: []*common.Consenter{
				{Host: "host1", Port: 7050},
				{Host: "host2", Port: 7050},
				{Host: "host3", Port: 7050},
			},
			expectAdded:   1,
			expectRemoved: 0,
			expectError:   false,
		},
		{
			name: "Remove one node",
			oldConsenters: []*common.Consenter{
				{Host: "host1", Port: 7050},
				{Host: "host2", Port: 7050},
			},
			newConsenters: []*common.Consenter{
				{Host: "host1", Port: 7050},
			},
			expectAdded:   0,
			expectRemoved: 1,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := compareConsenters(tt.oldConsenters, tt.newConsenters)
			assert.Equal(t, tt.expectAdded, len(added))
			assert.Equal(t, tt.expectRemoved, len(removed))
		})
	}
}

func TestValidateConsenterCerts(t *testing.T) {
	validCert := []byte(`-----BEGIN CERTIFICATE-----
MIIBajCCAQ+gAwIBAgIRAKY0ycrAK1sqeNJnfuhoj2swCgYIKoZIzj0EAwIwFjEU
MBIGA1UEChMLZXhhbXBsZS5jb20wHhcNMjMwNTExMTYwMDAwWhcNMjQwNTExMTYw
MDAwWjAWMRQwEgYDVQQKEwtleGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49
AwEHA0IABKr1f+YGQVPjwV8VXj1HSqFbT0DJBJUYxT/YgJLxCYLqoGBZu+Fk3ZCY
TMXZqGv9hV8UX2J1Lxh5GjWv7yAP4JmjTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNV
HRMBAf8EAjAAMCsGA1UdIwQkMCKAIIuQwVmTy5X0mKX7TZcPK5qK8t0aPWOKDm6B
B/7LjqU5MAoGCCqGSM49BAMCA0kAMEYCIQD6WKBuYqgwPWoQS5uUHEEXzpJH0Y0D
JxO9YH0EJ/yWXQIhAKxJbTpL5ThWYHbE6ECpF7eKFAhBsNhB1OCmO0M9YF1T
-----END CERTIFICATE-----`)

	emptyCert := []byte{}

	tests := []struct {
		name      string
		consenter *common.Consenter
		wantErr   bool
	}{
		{
			name: "Valid certificates",
			consenter: &common.Consenter{
				ServerTlsCert: validCert,
				ClientTlsCert: validCert,
			},
			wantErr: false,
		},
		{
			name: "Empty server cert",
			consenter: &common.Consenter{
				ServerTlsCert: emptyCert,
				ClientTlsCert: validCert,
			},
			wantErr: true,
		},
		{
			name: "Empty client cert",
			consenter: &common.Consenter{
				ServerTlsCert: validCert,
				ClientTlsCert: emptyCert,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConsenterCerts(tt.consenter)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
