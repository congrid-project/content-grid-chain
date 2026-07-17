package main

import (
	"fmt"
	"os"

	"content-grid-chain/app"

	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
)

func main() {
	const envPrefix = "CONTENT_GRID"

	rootCmd := NewRootCmd()
	if err := svrcmd.Execute(rootCmd, envPrefix, resolveHomeArg(os.Args[1:], app.DefaultNodeHome)); err != nil {
		fmt.Fprintln(rootCmd.ErrOrStderr(), err)
		os.Exit(1)
	}
}
