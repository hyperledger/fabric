package reconciler

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/protos"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"strings"
	"time"
)

var logger = flogging.MustGetLogger("reconcileCmd")

var (
	channelID string
	timeout   time.Duration
)

const (
	EndorserNotRequired bool = false
	OrdererNotRequired  bool = false
	PeerDeliverRequired bool = true
)

// Cmd returns the cobra command for Node
func Cmd(cf *ReconcileCmdFactory) *cobra.Command {
	reconcileCmd.AddCommand(reconcileBlockCmd(cf))
	return reconcileCmd
}

// ReconcileCmdFactory holds the clients used by ReconcileCmdFactory
type ReconcileCmdFactory struct {
	EndorserClient   pb.EndorserClient
	Signer           msp.SigningIdentity
	BroadcastClient  common.BroadcastClient
	DeliverClient    reconClientIntf
	BroadcastFactory BroadcastClientFactory
}

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Reconcile block on channel: block|txn",
	Long:  "Reconcile a specific block or transaction on channel: block|txn.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		common.InitCmd(cmd, args)
		//common.SetOrdererEnv(cmd, args)
	},
}

var flags *pflag.FlagSet

func init() {
	resetFlags()
}

// Explicitly define a method to facilitate tests
func resetFlags() {
	flags = &pflag.FlagSet{}

	flags.StringVarP(&channelID, "channelID", "c", common.UndefinedParamValue, "Channel ID is mandatory, as the block can be reconciled with in a channel")
	flags.DurationVarP(&timeout, "timeout", "t", 10*time.Second, "Reconcile response timeout")
}

func attachFlags(cmd *cobra.Command, names []string) {
	cmdFlags := cmd.Flags()
	for _, name := range names {
		if flag := flags.Lookup(name); flag != nil {
			cmdFlags.AddFlag(flag)
		} else {
			logger.Fatalf("Could not find flag '%s' to attach to commond '%s'", name, cmd.Name())
		}
	}
}

type BroadcastClientFactory func() (common.BroadcastClient, error)

type reconClientIntf interface {
	ReconcileSpecifiedBlock(num uint64) (*protos.ReconcileResponse, error)
	Close() error
}

// InitCmdFactory init the ChannelCmdFactory with clients to endorser and orderer according to params
func InitCmdFactory(isEndorserRequired, isPeerDeliverRequired, isOrdererRequired bool) (*ReconcileCmdFactory, error) {
	if isPeerDeliverRequired && isOrdererRequired {
		// this is likely a bug during development caused by adding a new cmd
		return nil, errors.New("ERROR - only a single deliver source is currently supported")
	}

	var err error
	cf := &ReconcileCmdFactory{}

	cf.Signer, err = common.GetDefaultSignerFnc()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting default signer")
	}

	cf.BroadcastFactory = func() (common.BroadcastClient, error) {
		return common.GetBroadcastClientFnc()
	}

	// for join and list, we need the endorser as well
	if isEndorserRequired {
		// creating an EndorserClient with these empty parameters will create a
		// connection using the values of "peer.address" and
		// "peer.tls.rootcert.file"
		cf.EndorserClient, err = common.GetEndorserClientFnc(common.UndefinedParamValue, common.UndefinedParamValue)
		if err != nil {
			return nil, errors.WithMessage(err, "error getting endorser client for channel")
		}
	}

	// for fetching blocks from a peer
	if isPeerDeliverRequired {
		cf.DeliverClient, err = common.NewP2PMessageClientForPeer(channelID, cf.Signer)
		if err != nil {
			return nil, errors.WithMessage(err, "error getting deliver client for channel")
		}
	}

	// for create and fetch, we need the orderer as well
	if isOrdererRequired {
		if len(strings.Split(common.OrderingEndpoint, ":")) != 2 {
			return nil, errors.Errorf("ordering service endpoint %s is not valid or missing", common.OrderingEndpoint)
		}
		cf.DeliverClient, err = common.NewP2PMessageClientForOrderer(channelID, cf.Signer)
		if err != nil {
			return nil, err
		}
	}

	logger.Infof("Endorser and orderer connections initialized")
	return cf, nil
}
