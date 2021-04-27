// main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/siisee11/golang-blockchain/blockchain"
)

// CommandLine은 BlockChain과 상호작용을 해야합니다.
type CommandLine struct {
	blockchain *blockchain.BlockChain
}

// Cli help 메세지 입니다.
func (cli *CommandLine) printUsage() {
	fmt.Println("Usage: ")
	fmt.Println(" add -block BLOCK_DATA - Add a block to the chain ")
	fmt.Println(" print - Prints the blocks in the chain")
}

// Args(arguments)가 1개면 명령어를 입력하지 않은 것이므로 종료합니다.
func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()

		// runtime.Goexit은 Go routine을 종료시키는 것이기 때문에
		// applicaion 강제 종료가 아니여서 DB가 정상 종료(close)될 수 있도록 해준다.
		runtime.Goexit()
	}
}

// AddBlock을 데이터를 담아 호출하여 새로운 블록을 만듭니다.
func (cli *CommandLine) addBlock(data string) {
	cli.blockchain.AddBlock(data)
	fmt.Println("Added Block!")
}

// Chain을 순회하며 블록을 출력합니다.
// LastHash 부터 Genesis순으로 출력합니다. (Iterator 구현을 기억!)
func (cli *CommandLine) printChain() {
	iter := cli.blockchain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Data in Block: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()

		// if Genesis
		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) run() {
	cli.validateArgs()

	// Go의 option 처리하는 함수들.
	addBlockCmd := flag.NewFlagSet("add", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("print", flag.ExitOnError)
	addBlockData := addBlockCmd.String("block", "", "Block data")

	switch os.Args[1] {
	case "add":
		err := addBlockCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	case "print":
		err := printChainCmd.Parse(os.Args[2:])
		blockchain.Handle(err)
	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if addBlockCmd.Parsed() {
		if *addBlockData == "" {
			addBlockCmd.Usage()
			runtime.Goexit()
		}
		cli.addBlock(*addBlockData)
	}

	if printChainCmd.Parsed() {
		cli.printChain()
	}
}

func main() {
	defer os.Exit(0)
	// Blockchain을 초기화 한다. 이는 Genesis block을 만드는 작업을 포함한다.
	chain := blockchain.InitBlockChain()
	defer chain.Database.Close()

	cli := CommandLine{chain}
	cli.run()
}
