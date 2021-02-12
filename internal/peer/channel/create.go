/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// ConfigTxFileNotFound channel create configuration tx file not found
type ConfigTxFileNotFound string

func (e ConfigTxFileNotFound) Error() string {
	return fmt.Sprintf("channel create configuration tx file not found %s", string(e))
}

// InvalidCreateTx invalid channel create transaction
type InvalidCreateTx string

func (e InvalidCreateTx) Error() string {
	return fmt.Sprintf("Invalid channel create transaction : %s", string(e))
}

func createCmd(cf *ChannelCmdFactory) *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a channel",
		Long:  "Create a channel and write the genesis block to a file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return create(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
		"file",
		"outputBlock",
		"timeout",
	}
	attachFlags(createCmd, flagList)

	return createCmd
}

func createChannelFromDefaults(cf *ChannelCmdFactory) (*cb.Envelope, error) {
	chCrtEnv, err := encoder.MakeChannelCreationTransaction(
		channelID,
		cf.Signer,
		genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile),
	)
	if err != nil {
		return nil, err
	}

	return chCrtEnv, nil
}

func createChannelFromConfigTx(configTxFileName string) (*cb.Envelope, error) {
	cftx, err := ioutil.ReadFile(configTxFileName)
	if err != nil {
		return nil, ConfigTxFileNotFound(err.Error())
	}

	return protoutil.UnmarshalEnvelope(cftx)
}

func sanityCheckAndSignConfigTx(envConfigUpdate *cb.Envelope, signer identity.SignerSerializer) (*cb.Envelope, error) {
	payload, err := protoutil.UnmarshalPayload(envConfigUpdate.Payload)
	if err != nil {
		return nil, InvalidCreateTx("bad payload")
	}

	if payload.Header == nil || payload.Header.ChannelHeader == nil {
		return nil, InvalidCreateTx("bad header")
	}

	ch, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, InvalidCreateTx("could not unmarshall channel header")
	}

	if ch.Type != int32(cb.HeaderType_CONFIG_UPDATE) {
		return nil, InvalidCreateTx("bad type")
	}

	if ch.ChannelId == "" {
		return nil, InvalidCreateTx("empty channel id")
	}

	// Specifying the chainID on the CLI is usually redundant, as a hack, set it
	// here if it has not been set explicitly
	if channelID == "" {
		channelID = ch.ChannelId
	}

	if ch.ChannelId != channelID {
		return nil, InvalidCreateTx(fmt.Sprintf("mismatched channel ID %s != %s", ch.ChannelId, channelID))
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	if err != nil {
		return nil, InvalidCreateTx("Bad config update env")
	}

	sigHeader, err := protoutil.NewSignatureHeader(signer)
	if err != nil {
		return nil, err
	}

	configSig := &cb.ConfigSignature{
		SignatureHeader: protoutil.MarshalOrPanic(sigHeader),
	}

	configSig.Signature, err = signer.Sign(util.ConcatenateBytes(configSig.SignatureHeader, configUpdateEnv.ConfigUpdate))
	if err != nil {
		return nil, err
	}

	configUpdateEnv.Signatures = append(configUpdateEnv.Signatures, configSig)

	return protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, channelID, signer, configUpdateEnv, 0, 0)
}

func sendCreateChainTransaction(cf *ChannelCmdFactory) error {
	var err error
	var chCrtEnv *cb.Envelope

	if channelTxFile != "" {
		if chCrtEnv, err = createChannelFromConfigTx(channelTxFile); err != nil {
			return err
		}
	} else {
		if chCrtEnv, err = createChannelFromDefaults(cf); err != nil {
			return err
		}
	}

	if chCrtEnv, err = sanityCheckAndSignConfigTx(chCrtEnv, cf.Signer); err != nil {
		return err
	}

	var broadcastClient common.BroadcastClient
	broadcastClient, err = cf.BroadcastFactory()
	if err != nil {
		return errors.WithMessage(err, "error getting broadcast client")
	}

	defer broadcastClient.Close()
	err = broadcastClient.Send(chCrtEnv)

	return err
}

func executeCreate(cf *ChannelCmdFactory) error {
	err := sendCreateChainTransaction(cf)
	if err != nil {
		return err
	}

	block, err := getGenesisBlock(cf)
	if err != nil {
		return err
	}

	b, err := proto.Marshal(block)
	if err != nil {
		return err
	}

	file := channelID + ".block"
	if outputBlock != common.UndefinedParamValue {
		file = outputBlock
	}
	err = ioutil.WriteFile(file, b, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func getGenesisBlock(cf *ChannelCmdFactory) (*cb.Block, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			cf.DeliverClient.Close()
			return nil, errors.New("timeout waiting for channel creation")
		default:
			if block, err := cf.DeliverClient.GetSpecifiedBlock(0); err != nil {
				cf.DeliverClient.Close()
				cf, err = InitCmdFactory(EndorserNotRequired, PeerDeliverNotRequired, OrdererRequired)
				if err != nil {
					return nil, errors.WithMessage(err, "failed connecting")
				}
				time.Sleep(200 * time.Millisecond)
			} else {
				cf.DeliverClient.Close()
				return block, nil
			}
		}
	}
}

func create(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	// the global chainID filled by the "-c" command
	if channelID == common.UndefinedParamValue {
		return errors.New("must supply channel ID")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserNotRequired, PeerDeliverNotRequired, OrdererRequired)
		if err != nil {
			return err
		}
	}
	return executeCreate(cf)
}
