package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/cli/bundle"
	"github.com/cloudflare/cfssl/cli/certinfo"
	"github.com/cloudflare/cfssl/cli/gencert"
	"github.com/cloudflare/cfssl/cli/gencrl"
	"github.com/cloudflare/cfssl/cli/genkey"
	"github.com/cloudflare/cfssl/cli/info"
	"github.com/cloudflare/cfssl/cli/ocspdump"
	"github.com/cloudflare/cfssl/cli/ocsprefresh"
	"github.com/cloudflare/cfssl/cli/ocspserve"
	"github.com/cloudflare/cfssl/cli/ocspsign"
	"github.com/cloudflare/cfssl/cli/printdefault"
	"github.com/cloudflare/cfssl/cli/revoke"
	"github.com/cloudflare/cfssl/cli/scan"
	"github.com/cloudflare/cfssl/cli/selfsign"
	"github.com/cloudflare/cfssl/cli/serve"
	"github.com/cloudflare/cfssl/cli/sign"
	"github.com/cloudflare/cfssl/cli/version"
	cenroll "github.com/hyperledger/fabric/cop/cli/cop/client/enroll"
	cregister "github.com/hyperledger/fabric/cop/cli/cop/client/register"
	"github.com/hyperledger/fabric/cop/cli/cop/server"

	_ "github.com/go-sql-driver/mysql" // import to support MySQL
	_ "github.com/lib/pq"              // import to support Postgres
	_ "github.com/mattn/go-sqlite3"    // import to support SQLite3
)

var usage = `cop client       - client-related commands
cop server       - server related commands
cop cfssl        - all cfssl commands

For help, type "cop client", "cop server", or "cop cfssl".
`

// COPMain is the COP main
func COPMain(args []string) int {
	if len(args) <= 1 {
		fmt.Println(usage)
		return 1
	}
	flag.Usage = nil // this is set to nil for testabilty
	cmd := os.Args[1]
	os.Args = os.Args[1:]
	switch cmd {
	case "client":
		clientCommand()
	case "server":
		serverCommand()
	case "cfssl":
		cfsslCommand()
	default:
		fmt.Println(usage)
		return 1
	}

	return 0
}

func clientCommand() {
	cmds := map[string]*cli.Command{
		"register": cregister.Command,
		"enroll":   cenroll.Command,
	}
	// If the CLI returns an error, exit with an appropriate status code.
	err := cli.Start(cmds)
	if err != nil {
		os.Exit(1)
	}
}

func serverCommand() {
	// The server commands
	cmds := map[string]*cli.Command{
		"start": server.StartCommand,
	}
	// Set the authentication handler
	serve.SetWrapHandler(server.NewAuthWrapper)
	// Add the "register" route/endpoint
	serve.SetEndpoint("register", server.NewRegisterHandler)
	// If the CLI returns an error, exit with an appropriate status code.
	err := cli.Start(cmds)
	if err != nil {
		os.Exit(1)
	}
}

func cfsslCommand() {
	cmds := map[string]*cli.Command{
		"bundle":         bundle.Command,
		"certinfo":       certinfo.Command,
		"sign":           sign.Command,
		"serve":          serve.Command,
		"version":        version.Command,
		"genkey":         genkey.Command,
		"gencert":        gencert.Command,
		"gencrl":         gencrl.Command,
		"ocspdump":       ocspdump.Command,
		"ocsprefresh":    ocsprefresh.Command,
		"ocspsign":       ocspsign.Command,
		"ocspserve":      ocspserve.Command,
		"selfsign":       selfsign.Command,
		"scan":           scan.Command,
		"info":           info.Command,
		"print-defaults": printdefaults.Command,
		"revoke":         revoke.Command,
	}

	// Replace "cfssl" with "cop cfssl" in all usage messages
	for _, cmd := range cmds {
		cmd.UsageText = strings.Replace(cmd.UsageText, "cfssl", "cop cfssl", -1)
	}

	// If the CLI returns an error, exit with an appropriate status code.
	err := cli.Start(cmds)
	if err != nil {
		os.Exit(1)
	}

}

func main() {
	os.Exit(COPMain(os.Args))
}
