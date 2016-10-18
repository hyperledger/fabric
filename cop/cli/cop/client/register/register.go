package register

import "github.com/cloudflare/cfssl/cli"

var usageText = `cop client register -- Register an ID with COP server and return an enrollment secret

Usage of client register command:
    Register a client with COP server:
        cop client register ID COP-SERVER-ADDR  // TODO: modify

Arguments:
        ID:               Enrollment ID
        COP-SERVER-ADDR:  COP server address

Flags:
`

var flags = []string{}

func myMain(args []string, c cli.Config) error {

	id, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	copServer, args, err := cli.PopFirstArgument(args)
	if err != nil {
		return err
	}

	//client := cop.NewClient()
	//client.Register
	_ = id
	_ = copServer
	_ = args

	return nil
}

// Command assembles the definition of Command 'enroll'
var Command = &cli.Command{UsageText: usageText, Flags: flags, Main: myMain}
