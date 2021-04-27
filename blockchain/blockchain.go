package blockchain

import (
	"fmt"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks"
)

// Badger DB를 고려해서 BlockChain을 재설계
// 기존의 Block slice는 메모리에 상주하기 때문에 프로그램 종료시 없어짐.
// DB를 가르키는 포인터를 저장해서 포인터를 통해 블록 관리
type BlockChain struct {
	LastHash []byte     // 마지막 블록의 hash
	Database *badger.DB // Badger DB를 가르키는 포인터
}

// BlockChain DB의 Block을 순회하는 자료구조
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Blockchain이 DB에 저장되어 있다면 불러오고,
// 없다면 새로 만들어 반환하는 함수
func InitBlockChain() *BlockChain {
	var lastHash []byte

	// File명을 통해 DB를 엽니다.
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	Handle(err)

	// db.Update는 Read/Write함수, View는 Read Only 함수입니다.
	// 수정사항(Genesis 생성)이 있기 때문에 Update함수를 사용합니다.
	err = db.Update(func(txn *badger.Txn) error {
		// Txn(transaction) closure

		// "lh"(lash hash)로 검색했는데 키가 발견되지 않았다면 저장이 안되어 있는것.
		if _, err := txn.Get([]byte("lh")); err == badger.ErrKeyNotFound {
			fmt.Println("No existing blockchain found")
			genesis := Genesis()
			fmt.Println("Genesis proved")

			// Key{genesis.Hash}, Value{genesis.Serialize()}를 넣습니다.
			// Serialize()함수로 block을 []byte로 바꾸어 저장합니다.
			err = txn.Set(genesis.Hash, genesis.Serialize())
			Handle(err)

			// "lh" (마지막 해시)도 저장합니다.
			err = txn.Set([]byte("lh"), genesis.Hash)
			lastHash = genesis.Hash

			return err
		} else {
			// "lh"가 검색이되면 블록체인이 저장되어 있는 것.
			item, err := txn.Get([]byte("lh"))
			Handle(err)
			lastHash, err = item.ValueCopy(nil)
			return err
		}
	})

	Handle(err)

	// 마지막 해시와 db pointer를 인자로하여 블록체인을 생성합니다.
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// 새로운 블록을 만들어서 블록체인에 연결하는 함수
func (chain *BlockChain) AddBlock(data string) {
	var lastHash []byte

	// Read만 하므로 View를 사용
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)

		return err
	})
	Handle(err)

	// lashHash를 토대로 다음 문제를 풀어 새로운 블록을 생성.
	newBlock := CreateBlock(data, lastHash)

	// 블록의 해시를 키값으로 새로운 블록을 저장하고
	// lh의 값 또한 새로운 블록의 해시로 업데이트 해줍니다.
	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})
	Handle(err)
}

// 아래 함수들은 Block을 iteration할 수 있도록 도와주는
// Iterator 관련 함수입니다.
// 아래 함수는 BlockChainIterator를 생성하여 반환합니다.
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

// Iterator는 순회를 목적으로 하기때문에
// 다음 객체를 반환하는 것이 중요합니다.
// Next()함수는 최신 블록에서 Genesis블록 쪽으로
// 다음 블록을 탐색해 포인터를 반환합니다.
func (iter *BlockChainIterator) Next() *Block {
	var block *Block

	// 현재 해시값 {CurrentHash}로 블록을 검색합니다.
	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		Handle(err)
		encodedBlock, err := item.ValueCopy(nil)
		block = Deserialize(encodedBlock)

		return err
	})
	Handle(err)

	// block에 저장된 PrevHash를 가져와서
	// 다음 탐색에 사용합니다.
	iter.CurrentHash = block.PrevHash

	return block
}
