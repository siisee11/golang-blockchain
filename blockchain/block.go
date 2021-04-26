// blockchain/block.go
package blockchain

type BlockChain struct {
	// BlockChain은 Block포인터 슬라이스를 가진다.
	Blocks []*Block
}

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

// 새로운 블록을 만들어서 블록체인에 연결하는 함수
func (chain *BlockChain) AddBlock(data string) {
	prevBlock := chain.Blocks[len(chain.Blocks)-1]
	new := CreateBlock(data, prevBlock.Hash)
	chain.Blocks = append(chain.Blocks, new)
}

// Chain의 첫 블록을 Genesis Block이라고 한다.
// Genesis Block은 이전 해시가 없으므로 예외처리한다.
func Genesis() *Block {
	return CreateBlock("Genesis", []byte{})
}

func InitBlockChain() *BlockChain {
	return &BlockChain{[]*Block{Genesis()}}
}
