// main.go
package main

import (
	"context"
	"os"

	"github.com/siisee11/golang-blockchain/cli"
)

func main() {
	_, cancle := context.WithCancel(context.Background())
	defer cancle()
	defer os.Exit(0)
	cmd := cli.CommandLine{}
	cmd.Run()
}
