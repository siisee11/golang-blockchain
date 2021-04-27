// blockchain/block.go
package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
)

// Block의 구조
type Block struct {
	Hash     []byte // 현재 블록의 해시
	Data     []byte // 블록에 기록된 data
	PrevHash []byte // 이전 블록의 해시
	Nonce    int
}

// Block을 생성하는 함수
// data와 이전 해시값을 인자로 받는다.
func CreateBlock(data string, prevHash []byte) *Block {
	// data와 이전 해시값으로 block을 만들고
	block := &Block{[]byte{}, []byte(data), prevHash, 0}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// Chain의 첫 블록을 Genesis Block이라고 한다.
// Genesis Block은 이전 해시가 없으므로 예외처리한다.
func Genesis() *Block {
	return CreateBlock("Genesis", []byte{})
}

// Badger DB가 arrays of byte 밖에 수용하지 못하기 때문에
// Block data structure를 serialize, deserialize해줄
// Util함수가 필요하다.
func (b *Block) Serialize() []byte {
	var res bytes.Buffer
	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)
	Handle(err)

	return res.Bytes()
}

func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)
	Handle(err)

	return &block
}

func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
