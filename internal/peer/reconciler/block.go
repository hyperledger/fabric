package reconciler

import (
	"fmt"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"strconv"
)

func reconcileBlockCmd(cf *ReconcileCmdFactory) *cobra.Command {
	reconcileBlockCmd := &cobra.Command{
		Use:   "block",
		Short: "reconcile on a block of a specified channel.",
		Long:  "get blockchain information of a specified channel. Requires '-c'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reconcileBlock(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
	}
	attachFlags(reconcileBlockCmd, flagList)

	return reconcileBlockCmd
}

func reconcileBlock(cmd *cobra.Command, args []string, rf *ReconcileCmdFactory) error {

	logger.Debugf("Received the arguments for reconcilation %d", args)

	if len(args) == 0 {
		return fmt.Errorf("reconcile target required, block number")
	}
	if len(args) > 1 {
		return fmt.Errorf("trailing args detected")
	}

	blockNumber, err2 := strconv.Atoi(args[0])
	if err2 != nil {
		return fmt.Errorf("fetch target illegal: %s", args[0])
	}

	if blockNumber < 0 {
		return fmt.Errorf("block number must be above 0")
	}

	// the global chainID filled by the "-c" command
	if channelID == common.UndefinedParamValue {
		return fmt.Errorf("must supply channel ID")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error

	if rf == nil {
		rf, err = InitCmdFactory(EndorserNotRequired, PeerDeliverRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	response, err := rf.DeliverClient.ReconcileSpecifiedBlock(uint64(blockNumber))

	if err != nil {
		fmt.Printf("Failed to reconcile block %d: %v\n", blockNumber, err.Error())
	} else {
		fmt.Println(response)
	}

	// fmt.Printf("Received the arguments for reconcilation %v on channel %s\n", args, channelID)

	err = rf.DeliverClient.Close()
	if err != nil {
		return err
	}

	return nil
}
