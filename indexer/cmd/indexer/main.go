package main

import (
	"context"
	"os"

	"github.com/ethereum/go-ethereum/log"

	oplog "github.com/BLASTchain/blast/bl-service/log"
	"github.com/BLASTchain/blast/bl-service/opio"
)

var (
	GitCommit = ""
	GitDate   = ""
)

func main() {
	oplog.SetupDefaults()
	app := newCli(GitCommit, GitDate)
	// sub-commands set up their individual interrupt lifecycles, which can block on the given interrupt as needed.
	ctx := opio.WithInterruptBlocker(context.Background())
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Error("application failed", "err", err)
		os.Exit(1)
	}
}
