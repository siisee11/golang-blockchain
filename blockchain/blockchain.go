package blockchain

import (
	"encoding/hex"
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
	db, err := badger.Open(badger.DefaultOptions(dbPath))
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
	db, err := badger.Open(badger.DefaultOptions(dbPath))
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
func (chain *BlockChain) AddBlock(transactions []*Transaction) {
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
}

// 아래 함수는 BlockChainIterator를 생성하여 반환합니다.
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

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

// UTXO가 포함된 모든 트랜잭션을 반환합니다.
func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspentTxs []Transaction

	// 사용된 TXO의 (txID => []Out) 매핑입니다.
	// "{txID}를 가진 트랜잭션의 {[]Out}번째 TXO들은 사용되었다."
	// TxInput에 속한 TXO는 사용된 TXO임을 기억하세요.
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	// 가장 최신의 블록부터 for loop를 수행합니다.
	for {
		block := iter.Next()

		// Block에 저장된 트랜잭션에 대해 for loop 수행
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			// 트랜잭션의 모든 TXO에대해 for loop
			for outIdx, out := range tx.Outputs {
				// {txID}의 트랜잭션에서 TXO가 사용된 기록이 있고
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						// {spentOut}: 사용된 TXO의 index
						if spentOut == outIdx {
							// 사용된 TXO의 Index와 같은 outIdx를 가지는
							// TXO는 사용된 TXO이므로 다음 TXO 조사
							continue Outputs
						}
					}
				}

				// 사용된 기록이 없고, address 소유이면
				// 사용되지 않은 트랜잭션에 추가합니다.
				if out.CanBeUnlocked(address) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}

			// 해당 트랜잭션이 coinbase가 아니라면 (일반 트랜잭션이라면)
			if !tx.IsCoinbase() {
				// 트랜잭션의 input 중에
				for _, in := range tx.Inputs {
					// address 소유인 것들은 사용한 TXO이므로 사용된 TXO 매핑에 추가합니다.
					if in.CanUnlock(address) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		// Genesis 까지 for를 돌았다면 break 합니다.
		if len(block.PrevHash) == 0 {
			break
		}
	}

	// {address}의 사용되지않은 트랜잭션을 반환합니다.
	// TXO가 아닌 UTXO가 포함된 트랜잭션이 반환됨을 유의합니다.
	return unspentTxs
}

func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var UTXOs []TxOutput // Unspent Transaction Outputs
	unspentTransactions := chain.FindUnspentTransactions(address)

	// 사용되지 않은 트랜잭션들의 Output(UTXO)중에
	// 나{address}의 UTXO들을 저장하여 반환.
	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}

	return UTXOs
}

// amount를 집불하기위해 사용될 UTXO를 검색합니다.
// 사용할 수 있는 금액과 사용할 수 있는 TXO를 찾을 수 있는 매핑을 반환합니다.
func (chain *BlockChain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	// UTXO의 txID => outIdx(해당 트랜잭션에서 몇번째 TXO가 UTXO인지) 매핑
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	// {address}의 사용되지 않은 트랜잭션들에 대해 for loop
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)

		// 트랜잭션의 모든 Output(TXO)에 대해 for loop
		for outIdx, out := range tx.Outputs {
			// {address}소유이고 지금까지의 UTXO의 합이 amount보다 작다면 해당 UTXO를 추가
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)

				// amount를 지불하기 위한 UTXO를 충분히 모았다면 for 종료
				if accumulated >= amount {
					break Work // Work로 레이블된 for loop 탈출
				}
			}
		}
	}

	return accumulated, unspentOuts
}
