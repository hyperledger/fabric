package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/log"
)

// Config is COP config structure
type Config struct {
	Debug          bool             `json:"debug,omitempty"`
	Authentication bool             `json:"authentication,omitempty"`
	Users          map[string]*User `jon:"users,omitempty"`
}

// User information
type User struct {
	Pass string `json:"pass"` // enrollment secret
}

// Constructor for COP config
func newConfig() *Config {
	c := new(Config)
	c.Authentication = true
	return c
}

// CFG is the COP-specific config
var CFG *Config

// Init initializes the COP config given the CFSSL config
func Init(cfg *cli.Config) {
	CFG = newConfig()
	if cfg.ConfigFile != "" {
		body, err := ioutil.ReadFile(cfg.ConfigFile)
		if err != nil {
			panic(err.Error())
		}
		err = json.Unmarshal(body, CFG)
		if err != nil {
			panic(fmt.Sprintf("error parsing %s: %s", cfg.ConfigFile, err.Error()))
		}
	}
	dbg := os.Getenv("COP_DEBUG")
	if dbg != "" {
		CFG.Debug = dbg == "true"
	}
	if CFG.Debug {
		log.Level = log.LevelDebug
	}
}
