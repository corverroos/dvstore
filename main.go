package main

import (
	"github.com/corverroos/dvstore/cmd"
	"github.com/spf13/cobra"
)

func main() {
	cobra.CheckErr(cmd.New().Execute())
}
