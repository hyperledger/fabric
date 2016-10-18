package enroll

import (
	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/log"
	"github.com/hyperledger/fabric/cop"
	"github.com/hyperledger/fabric/cop/cli/cop/config"
)

var usageText = `cop client enroll -- Enroll with COP server

Usage of client enroll command:
    Enroll a client and get an ecert:
        cop client enroll ID SECRET COP-SERVER-ADDR

Arguments:
        ID:               Enrollment ID
        SECRET:           Enrollment secret returned by register
        CSRJSON:          Certificate Signing Request JSON information
        COP-SERVER-ADDR:  COP server address

Flags:
`

var flags = []string{}

func myMain(args []string, c cli.Config) error {

	config.Init(&c)

	log.Debug("in myMain of 'cop client enroll'")

	id, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	secret, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	csrJSON, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	copServer, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	_ = args

	client := cop.NewClient()
	_, err = client.Enroll(id, secret, copServer, csrJSON)

	return err
}

// Command assembles the definition of Command 'enroll'
var Command = &cli.Command{UsageText: usageText, Flags: flags, Main: myMain}
