// Package main は awsp バイナリのエントリポイント
package main

import (
	"os"

	"github.com/kagamirror123/awsp/cmd"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, commit, date)

	os.Exit(cmd.Execute())
}
