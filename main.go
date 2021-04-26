// main.go
package main

import (
	"fmt"

	"github.com/siisee11/golang-blockchain/blockchain"
)

func main() {
	// Blockchain을 초기화 한다. 이는 Genesis block을 만드는 작업을 포함한다.
	chain := blockchain.InitBlockChain()

	// 예시로 3개의 블록을 추가한다.
	chain.AddBlock("First Block after Genesis")
	chain.AddBlock("second Block after Genesis")
	chain.AddBlock("Third Block after Genesis")

	// Block을 iterate하며 출력한다.
	for _, block := range chain.Blocks {
		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Data in Block: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)
	}
}
