//blockchain/chain_iter.go
package blockchain

import "github.com/dgraph-io/badger"

// BlockChain DB의 Block을 순회하는 자료구조
type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
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
