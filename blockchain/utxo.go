package blockchain

import (
	"bytes"
	"encoding/hex"
	"log"

	"github.com/dgraph-io/badger"
)

// utxo- 로 prefix된 key를 위한 상수 입니다.
var (
	utxoPrefix   = []byte("utxo-")
	prefixLength = len(utxoPrefix)
)

// UTXO를 관리하는 구조입니다.
// BlockChain의 Database를 공유하므로 BlockChain에 대한 포인터를 가집니다.
type UTXOSet struct {
	Blockchain *BlockChain
}

// blockchain.go 에서 옮겨온 함수.
// UTXOSet 중에 pubKeyHash 소유의 UTXO를 반환.
func (u UTXOSet) FindUnspentTransactions(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// "utxo-"로 prefix된 key를 찾아 value를 가져옵니다.
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			Handle(err)
			outs := DeserializeOutputs(v)

			// 결과값 중 pubKeyHash 소유의 UTXO만 저장합니다.
			for _, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})
	Handle(err)

	return UTXOs
}

// blockchain.go 에서 옮겨온 함수.
// amount를 집불하기위해 사용될 UTXO를 검색합니다.
// 사용할 수 있는 금액과 사용할 수 있는 TXO를 찾을 수 있는 매핑을 반환합니다.
func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	// UTXO의 txID => outIdx(해당 트랜잭션에서 몇번째 TXO가 UTXO인지) 매핑
	unspentOuts := make(map[string][]int)
	accumulated := 0

	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		// "utxo-"로 prefix된 key 모두 탐색합니다.
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			Handle(err)
			// "utxo-" prefix를 제거합니다.
			k = bytes.TrimPrefix(k, utxoPrefix)
			txID := hex.EncodeToString(k)
			outs := DeserializeOutputs(v)

			// 트랜잭션의 모든 Output(TXO)에 대해 for loop
			for outIdx, out := range outs.Outputs {
				// {pubKeyHash}소유이고 지금까지의 UTXO의 합이 amount보다 작다면 해당 UTXO를 추가
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOuts[txID] = append(unspentOuts[txID], outIdx)
				}
			}
		}

		return nil
	})
	Handle(err)

	return accumulated, unspentOuts
}

// "utxo-" prefix된 key를 지우고 새로 매핑한다.
func (u UTXOSet) Reindex() {
	db := u.Blockchain.Database

	// "utxo-" prefix된 데이터를 모두s 지운다.
	u.DeleteByPrefix(utxoPrefix)

	// UTXO를 모두 찾는다.
	UTXO := u.Blockchain.FindUTXO()

	err := db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO {
			key, err := hex.DecodeString(txId)
			if err != nil {
				return err
			}
			// 찾은 UTXO의 txId에 prefix를 더해 저장한다.
			key = append(utxoPrefix, key...)

			err = txn.Set(key, outs.Serialize())
			Handle(err)
		}

		return nil
	})
	Handle(err)
}

// AddBlock시에 발생합니다.
// block의 transaction을 DB에 업데이트
func (u *UTXOSet) Update(block *Block) {
	db := u.Blockchain.Database

	err := db.Update(func(txn *badger.Txn) error {
		for _, tx := range block.Transactions {
			if !tx.IsCoinbase() {
				// 트랜잭션의 모든 Input에 대하여
				for _, in := range tx.Inputs {
					updatedOuts := TxOutputs{}
					inID := append(utxoPrefix, in.ID...)
					// in.ID에 "utxo-" prefix를 붙혀 찾는다.
					item, err := txn.Get(inID)
					Handle(err)
					v, err := item.ValueCopy(nil)
					Handle(err)

					// outs은 in.ID 트랜잭션의 모든 UTXO
					outs := DeserializeOutputs(v)

					// UTXO 중에
					for outIdx, out := range outs.Outputs {
						// 이번 트랜잭션에서 사용할 UTXO 제외하고 (XXX: 아래 if문이 맞나?)
						if outIdx != in.Out {
							// 새로운 UTXO 배열을 만든다.
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					// UTXO가 남아있지 않다면 삭제.
					if len(updatedOuts.Outputs) == 0 {
						if err := txn.Delete(inID); err != nil {
							log.Panic(err)
						}
					} else {
						// UTXO가 남아있다면 다시 저장.
						if err := txn.Set(inID, updatedOuts.Serialize()); err != nil {
							log.Panic(err)
						}
					}
				}
			}
			// 이번 트랜잭션으로 생기는 UTXO들
			newOutputs := TxOutputs{}
			for _, out := range tx.Outputs {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}

			// "utxo-" prefix하여 UTXO들을 저장한다.
			txID := append(utxoPrefix, tx.ID...)
			if err := txn.Set(txID, newOutputs.Serialize()); err != nil {
				log.Panic(err)
			}
		}
		return nil
	})
	Handle(err)
}

// "utxo-" prefix key를 가진 트랜잭션의 수를 반환한다.
func (u UTXOSet) CountTransactions() int {
	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}

		return nil
	})

	Handle(err)
	return counter
}

// {prefix}가 붙은 Key-Value를 제거한다.
func (u *UTXOSet) DeleteByPrefix(prefix []byte) {
	// {KeysForDelete}에 속한 key-Value를 제거한다.
	deleteKeys := func(KeysForDelete [][]byte) error {
		if err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
			for _, key := range KeysForDelete {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	collectSize := 100000 // badger의 optimal batching size
	u.Blockchain.Database.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // value를 읽을 필요없으므로
		it := txn.NewIterator(opts)
		defer it.Close()

		keysForDelete := make([][]byte, 0, collectSize)
		keysCollected := 0
		// prefix된 모든 Key-value에 대해
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// key를 가져와 지울 키 리스트에 추가한다.
			key := it.Item().KeyCopy(nil)
			keysForDelete = append(keysForDelete, key)
			keysCollected++
			// batching size만큼 key를 모았으면
			if keysCollected == collectSize {
				// 해당 key값들을 지운다.
				if err := deleteKeys(keysForDelete); err != nil {
					log.Panic(err)
				}
				// 변수 초기화
				keysForDelete = make([][]byte, 0, collectSize)
				keysCollected = 0
			}
		}
		// 처리되지않은 키들을 마저 처리한다.
		if keysCollected > 0 {
			if err := deleteKeys(keysForDelete); err != nil {
				log.Panic(err)
			}
		}

		return nil
	})
}
