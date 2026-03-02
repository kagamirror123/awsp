// Package main は awsp バイナリのエントリポイント
package main

import (
	"os"

	"github.com/kagamirror123/awsp/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
