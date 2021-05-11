// blockchain/block.go
package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
	"time"
)

// Block의 구조
type Block struct {
	Timestamp    int64          // 블록생성 시각
	Hash         []byte         // 현재 블록의 해시
	Transactions []*Transaction // Data 대신 트랜잭션이 기록됨
	PrevHash     []byte         // 이전 블록의 해시
	Nonce        int
	Height       int // chain의 길이
}

// Block의 Transaction들을 합쳐서 하나의 해시를 만듭니다.
// Merkle Tree를 이용합니다.
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte

	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.Serialize())
	}
	tree := NewMerkleTree(txHashes)

	// merkle tree를 구성하고 루트노드의 데이터 값이 최종 해시값
	return tree.RootNode.Data
}

// Transaction과 이전 해시값을 인자로 받는다.
func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{time.Now().Unix(), []byte{}, txs, prevHash, 0, height}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Genesis Block은 coinbase 트랜잭션을 인자로 받습니다.
func Genesis(coinbase *Transaction) *Block {
	return CreateBlock([]*Transaction{coinbase}, []byte{}, 0)
}

// Util 함수
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)
	Handle(err)

	return res.Bytes()
}

// Util 함수
func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)
	Handle(err)

	return &block
}

// Util 함수
func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
