/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricconfig

import (
	"time"

	docker "github.com/fsouza/go-dockerclient"
)

type Core struct {
	Logging   *Logging   `yaml:"logging,omitempty"`
	Peer      *Peer      `yaml:"peer,omitempty"`
	VM        *VM        `yaml:"vm,omitempty"`
	Chaincode *Chaincode `yaml:"chaincode,omitempty"`
	Ledger    *Ledger    `yaml:"ledger,omitempty"`
	Metrics   *Metrics   `yaml:"metrics,omitempty"`
}

type Logging struct {
	Level  string `yaml:"level,omitempty"`
	Format string `yaml:"format,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Peer struct {
	ID                     string          `yaml:"id,omitempty"`
	NetworkID              string          `yaml:"networkId,omitempty"`
	ListenAddress          string          `yaml:"listenAddress,omitempty"`
	ChaincodeListenAddress string          `yaml:"chaincodeListenAddress,omitempty"`
	ChaincodeAddress       string          `yaml:"chaincodeAddress,omitempty"`
	Address                string          `yaml:"address,omitempty"`
	AddressAutoDetect      bool            `yaml:"addressAutoDetect"`
	Keepalive              *Keepalive      `yaml:"keepalive,omitempty"`
	Gossip                 *Gossip         `yaml:"gossip,omitempty"`
	Events                 *Events         `yaml:"events,omitempty"`
	TLS                    *TLS            `yaml:"tls,omitempty"`
	Authentication         *Authentication `yaml:"authentication,omitempty"`
	FileSystemPath         string          `yaml:"fileSystemPath,omitempty"`
	BCCSP                  *BCCSP          `yaml:"BCCSP,omitempty"`
	MSPConfigPath          string          `yaml:"mspConfigPath,omitempty"`
	LocalMSPID             string          `yaml:"localMspId,omitempty"`
	Deliveryclient         *DeliveryClient `yaml:"deliveryclient,omitempty"`
	LocalMspType           string          `yaml:"localMspType,omitempty"`
	AdminService           *Service        `yaml:"adminService,omitempty"`
	Handlers               *Handlers       `yaml:"handlers,omitempty"`
	ValidatorPoolSize      int             `yaml:"validatorPoolSize,omitempty"`
	Discovery              *Discovery      `yaml:"discovery,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Keepalive struct {
	MinInterval    time.Duration    `yaml:"minInterval,omitempty"`
	Client         *ClientKeepalive `yaml:"client,omitempty"`
	DeliveryClient *ClientKeepalive `yaml:"deliveryClient,omitempty"`
}

type ClientKeepalive struct {
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

type Gossip struct {
	Bootstrap                  string          `yaml:"bootstrap,omitempty"`
	UseLeaderElection          bool            `yaml:"useLeaderElection"`
	OrgLeader                  bool            `yaml:"orgLeader"`
	Endpoint                   string          `yaml:"endpoint,omitempty"`
	MaxBlockCountToStore       int             `yaml:"maxBlockCountToStore,omitempty"`
	MaxPropagationBurstLatency time.Duration   `yaml:"maxPropagationBurstLatency,omitempty"`
	MaxPropagationBurstSize    int             `yaml:"maxPropagationBurstSize,omitempty"`
	PropagateIterations        int             `yaml:"propagateIterations,omitempty"`
	PropagatePeerNum           int             `yaml:"propagatePeerNum,omitempty"`
	PullInterval               time.Duration   `yaml:"pullInterval,omitempty"`
	PullPeerNum                int             `yaml:"pullPeerNum,omitempty"`
	RequestStateInfoInterval   time.Duration   `yaml:"requestStateInfoInterval,omitempty"`
	PublishStateInfoInterval   time.Duration   `yaml:"publishStateInfoInterval,omitempty"`
	StateInfoRetentionInterval time.Duration   `yaml:"stateInfoRetentionInterval,omitempty"`
	PublishCertPeriod          time.Duration   `yaml:"publishCertPeriod,omitempty"`
	DialTimeout                time.Duration   `yaml:"dialTimeout,omitempty"`
	ConnTimeout                time.Duration   `yaml:"connTimeout,omitempty"`
	RecvBuffSize               int             `yaml:"recvBuffSize,omitempty"`
	SendBuffSize               int             `yaml:"sendBuffSize,omitempty"`
	DigestWaitTime             time.Duration   `yaml:"digestWaitTime,omitempty"`
	RequestWaitTime            time.Duration   `yaml:"requestWaitTime,omitempty"`
	ResponseWaitTime           time.Duration   `yaml:"responseWaitTime,omitempty"`
	AliveTimeInterval          time.Duration   `yaml:"aliveTimeInterval,omitempty"`
	AliveExpirationTimeout     time.Duration   `yaml:"aliveExpirationTimeout,omitempty"`
	ReconnectInterval          time.Duration   `yaml:"reconnectInterval,omitempty"`
	ExternalEndpoint           string          `yaml:"externalEndpoint,omitempty"`
	Election                   *GossipElection `yaml:"election,omitempty"`
	PvtData                    *GossipPvtData  `yaml:"pvtData,omitempty"`
}

type GossipElection struct {
	StartupGracePeriod       time.Duration `yaml:"startupGracePeriod,omitempty"`
	MembershipSampleInterval time.Duration `yaml:"membershipSampleInterval,omitempty"`
	LeaderAliveThreshold     time.Duration `yaml:"leaderAliveThreshold,omitempty"`
	LeaderElectionDuration   time.Duration `yaml:"leaderElectionDuration,omitempty"`
}

type GossipPvtData struct {
	PullRetryThreshold              time.Duration `yaml:"pullRetryThreshold,omitempty"`
	TransientstoreMaxBlockRetention int           `yaml:"transientstoreMaxBlockRetention,omitempty"`
	PushAckTimeout                  time.Duration `yaml:"pushAckTimeout,omitempty"`
}

type Events struct {
	Address    string        `yaml:"address,omitempty"`
	Buffersize int           `yaml:"buffersize,omitempty"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
	Timewindow time.Duration `yaml:"timewindow,omitempty"`
	Keepalive  *Keepalive    `yaml:"keepalive,omitempty"`
}

type TLS struct {
	Enabled            bool      `yaml:"enabled"`
	ClientAuthRequired bool      `yaml:"clientAuthRequired"`
	CA                 *FileRef  `yaml:"ca,omitempty"`
	Cert               *FileRef  `yaml:"cert,omitempty"`
	Key                *FileRef  `yaml:"key,omitempty"`
	RootCert           *FileRef  `yaml:"rootcert,omitempty"`
	ClientRootCAs      *FilesRef `yaml:"clientRootCAs,omitempty"`
	ClientKey          *FileRef  `yaml:"clientKey,omitempty"`
	ClientCert         *FileRef  `yaml:"clientCert,omitempty"`
}

type FileRef struct {
	File string `yaml:"file,omitempty"`
}

type FilesRef struct {
	Files []string `yaml:"files,omitempty"`
}

type Authentication struct {
	Timewindow time.Duration `yaml:"timewindow,omitempty"`
}

type BCCSP struct {
	Default string            `yaml:"Default,omitempty"`
	SW      *SoftwareProvider `yaml:"SW,omitempty"`
}

type SoftwareProvider struct {
	Hash     string `yaml:"Hash,omitempty"`
	Security int    `yaml:"Security,omitempty"`
}

type DeliveryClient struct {
	ReconnectTotalTimeThreshold time.Duration `yaml:"reconnectTotalTimeThreshold,omitempty"`
}

type Service struct {
	Enabled       bool   `yaml:"enabled"`
	ListenAddress string `yaml:"listenAddress,omitempty"`
}

type Handlers struct {
	AuthFilters []Handler  `yaml:"authFilters,omitempty"`
	Decorators  []Handler  `yaml:"decorators,omitempty"`
	Endorsers   HandlerMap `yaml:"endorsers,omitempty"`
	Validators  HandlerMap `yaml:"validators,omitempty"`
}

type Handler struct {
	Name    string `yaml:"name,omitempty"`
	Library string `yaml:"library,omitempty"`
}

type HandlerMap map[string]Handler

type Discovery struct {
	Enabled                      bool    `yaml:"enabled"`
	AuthCacheEnabled             bool    `yaml:"authCacheEnabled"`
	AuthCacheMaxSize             int     `yaml:"authCacheMaxSize,omitempty"`
	AuthCachePurgeRetentionRatio float64 `yaml:"authCachePurgeRetentionRatio"`
	OrgMembersAllowedAccess      bool    `yaml:"orgMembersAllowedAccess"`
}

type VM struct {
	Endpoint string  `yaml:"endpoint,omitempty"`
	Docker   *Docker `yaml:"docker,omitempty"`
}

type Docker struct {
	TLS          *TLS               `yaml:"tls,omitempty"`
	AttachStdout bool               `yaml:"attachStdout"`
	HostConfig   *docker.HostConfig `yaml:"hostConfig,omitempty"`
}

type Chaincode struct {
	Builder        string        `yaml:"builder,omitempty"`
	Pull           bool          `yaml:"pull"`
	Golang         *Golang       `yaml:"golang,omitempty"`
	Car            *Car          `yaml:"car,omitempty"`
	Java           *Java         `yaml:"java,omitempty"`
	Node           *Node         `yaml:"node,omitempty"`
	StartupTimeout time.Duration `yaml:"startupTimeout,omitempty"`
	ExecuteTimeout time.Duration `yaml:"executeTimeout,omitempty"`
	Mode           string        `yaml:"mode,omitempty"`
	Keepalive      int           `yaml:"keepalive,omitempty"`
	System         SystemFlags   `yaml:"system,omitempty"`
	Logging        *Logging      `yaml:"logging,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Golang struct {
	Runtime     string `yaml:"runtime,omitempty"`
	DynamicLink bool   `yaml:"dynamicLink"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Car struct {
	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Java struct {
	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Node struct {
	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type SystemFlags struct {
	CSCC string `yaml:"cscc,omitempty"`
	LSCC string `yaml:"lscc,omitempty"`
	ESCC string `yaml:"escc,omitempty"`
	VSCC string `yaml:"vscc,omitempty"`
	QSCC string `yaml:"qscc,omitempty"`
}

type Ledger struct {
	// Blockchain - not sure if it's needed
	State   *StateConfig   `yaml:"state,omitempty"`
	History *HistoryConfig `yaml:"history,omitempty"`
}

type StateConfig struct {
	StateDatabase string         `yaml:"stateDatabase,omitempty"`
	CouchDBConfig *CouchDBConfig `yaml:"couchDBConfig,omitempty"`
}

type CouchDBConfig struct {
	CouchDBAddress          string        `yaml:"couchDBAddress,omitempty"`
	Username                string        `yaml:"username,omitempty"`
	Password                string        `yaml:"password,omitempty"`
	MaxRetries              int           `yaml:"maxRetries,omitempty"`
	MaxRetriesOnStartup     int           `yaml:"maxRetriesOnStartup,omitempty"`
	RequestTimeout          time.Duration `yaml:"requestTimeout,omitempty"`
	QueryLimit              int           `yaml:"queryLimit,omitempty"`
	MaxBatchUpdateSize      int           `yaml:"maxBatchUpdateSize,omitempty"`
	WarmIndexesAfterNBlocks int           `yaml:"warmIndexesAfteNBlocks,omitempty"`
}

type HistoryConfig struct {
	EnableHistoryDatabase bool `yaml:"enableHistoryDatabase"`
}

type Metrics struct {
	Enabled        bool            `yaml:"enabled"`
	Reporter       string          `yaml:"reporter,omitempty"`
	Interval       time.Duration   `yaml:"interval,omitempty"`
	StatsdReporter *StatsdReporter `yaml:"statsdReporter,omitempty"`
	PromReporter   *Service        `yaml:"promReporter,omitempty"`
}

type StatsdReporter struct {
	Address       string        `yaml:"address,omitempty"`
	FlushInterval time.Duration `yaml:"flushInterval,omitempty"`
	FlushBytes    int           `yaml:"flushBytes,omitempty"`
}
