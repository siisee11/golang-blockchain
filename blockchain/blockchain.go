package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks_%s"
	dbFile = "MANIFEST"
)

// DB를 가르키는 포인터를 저장해서 포인터를 통해 블록 관리
type BlockChain struct {
	LastHash []byte     // 마지막 블록의 hash
	Database *badger.DB // Badger DB를 가르키는 포인터
}

// MANIFEST file 존재 여부로 DB 존재 확인
func DBexists(path string) bool {
	if _, err := os.Stat(path + "/" + dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Blockchain을 새로 만들어 반환하는 함수
func InitBlockChain(address, nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	var lastHash []byte

	if DBexists(path) {
		fmt.Println("Blockcahin already exists")
		runtime.Goexit()
	}

	// File명을 통해 DB를 엽니다.
	opts := badger.DefaultOptions(path)
	// log 무시
	opts.Logger = nil
	db, err := openDB(path, opts)
	Handle(err)

	// db.Update는 Read/Write함수, View는 Read Only 함수입니다.
	// 수정사항(Genesis 생성)이 있기 때문에 Update함수를 사용합니다.
	err = db.Update(func(txn *badger.Txn) error {
		// coinbase 트랜잭션을 만들어서, 이를 통해 Genesis block을 만들어 저장합니다.
		cbtx := CoinbaseTx(address, "")
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
func ContinueBlockChain(nodeId string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeId)
	if !DBexists(path) {
		fmt.Println("No existing blockchain found, create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	// File명을 통해 DB를 엽니다.
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := openDB(path, opts)
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

// {chain}에 {block}을 추가합니다.
// {block}이 이미 blockchain에 기록되어 있다면 skip합니다.
func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {
		// 블록이 이미 있다면 그냥 리턴
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData := block.Serialize()
		// 새로운 블록을 DB에 추가
		err := txn.Set(block.Hash, blockData)
		Handle(err)

		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		// local에 저장되어 있는 가장 최신블록 {lastBlock}
		lastBlock := Deserialize(lastBlockData)

		// 새로 받은 block의 Height가 더 높다면
		if block.Height > lastBlock.Height {
			// lh를 받은 블록의 해시값으로 업데이트합니다.
			err = txn.Set([]byte("lh"), block.Hash)
			Handle(err)
			chain.LastHash = block.Hash
		}

		return nil
	})
	Handle(err)
}

// lh에 해당하는 블록의 Height 반환.
func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock = *Deserialize(lastBlockData)

		return nil
	})
	Handle(err)

	return lastBlock.Height
}

// Block의 Hash값으로 블록 객체를 검색
func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block is not found")
		} else {
			blockData, _ := item.ValueCopy(nil)

			block = *Deserialize(blockData)
		}
		return nil
	})
	if err != nil {
		return block, err
	}

	return block, nil
}

// {chain}의 모든 블록의 해시값을 배열로 리턴합니다.
func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

// 새로운 블록을 채굴하여 블록체인에 연결하는 함수
// 새로 추가된 블록을 리턴함.
func (chain *BlockChain) MintBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	for _, tx := range transactions {
		if !chain.VerifyTransaction(tx) {
			log.Panic("Invalid Transaction")
		}
	}

	// 가장 최근 블록의 Hash가져옴
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(nil)
		Handle(err)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, _ := item.ValueCopy(nil)

		lastBlock := Deserialize(lastBlockData)

		lastHeight = lastBlock.Height

		return err
	})
	Handle(err)

	// lashHash를 토대로 다음 문제를 풀어 새로운 블록을 생성.
	newBlock := CreateBlock(transactions, lastHash, lastHeight+1)

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

func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}

func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
}
