package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis" // Genesis block의 시그니쳐 데이터
)

// DB를 가르키는 포인터를 저장해서 포인터를 통해 블록 관리
type BlockChain struct {
	LastHash []byte     // 마지막 블록의 hash
	Database *badger.DB // Badger DB를 가르키는 포인터
}

// MANIFEST file 존재 여부로 DB 존재 확인
func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Blockchain을 새로 만들어 반환하는 함수
func InitBlockChain(address string) *BlockChain {
	var lastHash []byte

	if DBexists() {
		fmt.Println("Blockcahin already exists")
		runtime.Goexit()
	}

	// File명을 통해 DB를 엽니다.
	opts := badger.DefaultOptions(dbPath)
	// log 무시
	opts.Logger = nil
	db, err := badger.Open(opts)
	Handle(err)

	// db.Update는 Read/Write함수, View는 Read Only 함수입니다.
	// 수정사항(Genesis 생성)이 있기 때문에 Update함수를 사용합니다.
	err = db.Update(func(txn *badger.Txn) error {
		// coinbase 트랜잭션을 만들어서, 이를 통해 Genesis block을 만들어 저장합니다.
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")
		err = txn.Set(genesis.Hash, genesis.Serialize())
		Handle(err)
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	Handle(err)

	// 마지막 해시와 db pointer를 인자로하여 블록체인을 생성합니다.
	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

// 이미 블록체인이 DB에 있으면 그 정보를 이용해서 *BlockChain을 반환합니다.
func ContinueBlockChain(address string) *BlockChain {
	if !DBexists() {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	// File명을 통해 DB를 엽니다.
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil
	db, err := badger.Open(opts)
	Handle(err)

	// 값을 가져오는 것이므로 View를 사용합니다.
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)

		return err
	})
	Handle(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

// 새로운 블록을 만들어서 블록체인에 연결하는 함수
// 새로 추가된 블록을 리턴함.
func (chain *BlockChain) AddBlock(transactions []*Transaction) *Block {
	var lastHash []byte

	// 가장 최근 블록의 Hash가져옴
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)

		return err
	})
	Handle(err)

	// lashHash를 토대로 다음 문제를 풀어 새로운 블록을 생성.
	newBlock := CreateBlock(transactions, lastHash)

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

	return newBlock
}

// block의 모든 UTXO (txID => TxOutputs mapping)을 찾아 반환합니다.
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXOs := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		// 블록의 모든 트랜잭션에 대하여
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				// outs에 UTXO추가
				outs := UTXOs[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXOs[txID] = outs
			}
			if !tx.IsCoinbase() {
				// Inputs에 참조된 모든 UTXO는 이번 트랜잭션에서 사용된 UTXO이다.
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return UTXOs
}

// Block을 순회하면서 Transaction ID를 가진 Transaction을 검색합니다.
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Equal(tx.ID, ID) {
				return *tx, nil
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction does not exist")
}

// 트랜잭션을 Private Key를 이용해 Sign합니다.
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	// 트랜잭션의 인풋에 대하여 for loop
	for _, in := range tx.Inputs {
		// input에 적힌 정보로 해당 UTXO의 이전 거래를 검색한다.
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	// 이전 거래 기록과 Private Key를 이용해 서명합니다.
	tx.Sign(privKey, prevTXs)
}

// 트랜잭션을 검증합니다.
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	// Coinbase 트랜잭션은 유효하다고 판단합니다.
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	// 이전 거래 기록을 이용해서 검증합니다.
	return tx.Verify(prevTXs)
}
