/**
* Copyright (c) 2018 EMSECO 
* This source code is being disclosed to you solely for the purpose of your participation in 
* testing EMSECO. You may view, compile and run the code for that purpose and pursuant to 
* the protocols and algorithms that are programmed into, and intended by, the code. You may 
* not do anything else with the code without express permission from EMSECO Foundation Ltd., 
* including modifying or publishing the code (or any part of it), and developing or forming 
* another public or private blockchain network. This source code is provided ‘as is’ and no 
* warranties are given as to title or non-infringement, merchantability or fitness for purpose 
* and, to the extent permitted by law, all liability for your use of the code is disclaimed. 
* Some programs in this code are governed by the GNU General Public License v3.0 (available at 
* https://www.gnu.org/licenses/gpl-3.0.en.html) (‘GPLv3’). The programs that are governed by 
* GPLv3.0 are those programs that are located in the folders src/depends and tests/depends 
* and which include a reference to GPLv3 in their program files.
**/

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"time"

	"go-hashgrid/core"
	"go-hashgrid/log"
	"go-hashgrid/node"
)

func main() {
	Duration := 1000

	// initialize
	gc := Initialize()
	log.Info("Main Initialized ...", "coinbase", gc.Coinbase, "peers", gc.Peers[0])
	log.Info("Hashgrid run ...")

	nd := node.NewNode(gc)
	go nd.EventHandle()
	for {
		time.Sleep(time.Duration(Duration) * time.Millisecond)
	}
}

func Initialize() *core.GlobalConfig {
	// Initialize the logger
	var verbosity = flag.Int("verbosity", int(log.LvlInfo), "log verbosity(0-9)")

	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(*verbosity))
	log.Root().SetHandler(glogger)

	// Initialize genesis.json
	gc := core.GlobalConfig{}
	parseit := NewParseConfig()
	err := parseit.Load("./config.json", &gc)
	if err != nil {
		log.Info("Load config.json failed")
	}

	log.Info("Initialize Success ...", "addr", gc.Addr)

	return &gc
}

type ParseConfig struct {
}

func NewParseConfig() *ParseConfig {
	return &ParseConfig{}
}

func (pc *ParseConfig) Load(filename string, v interface{}) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, v)
	if err != nil {
		log.Info("ParseConfig Load", "error", err)
		return err
	}

	return nil
}
