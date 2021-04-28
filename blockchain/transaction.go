package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

// Transaction은 Input, Output으로 구성되어있습니다.
type Transaction struct {
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

// 이것이 TXO(Transaction Output)입니다.
// "트랜잭션의 아웃풋"과 TXO라는 표현을 병행해서 사용합니다.
type TxOutput struct {
	Value int // 잔액

	// 소유자의 공개키
	// 여기서는 쉽게 소유자의 주소를 사용합니다.
	PubKey string
}

// 트랜잭션의 인풋은 이전 트랜잭션에서의 아웃풋을 사용하는 것임을 기억해야합니다.
// {ID}를 가지는 트랜잭션의 {OUT}번째 {Sig} 소유의 아웃풋으로 생각할 수 있습니다.
type TxInput struct {
	ID  []byte
	Out int
	Sig string // 소유자의 서명
}

// 1
// transaction에ID를 부여
func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encode := gob.NewEncoder(&encoded)
	err := encode.Encode(tx)
	Handle(err)

	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

// 1
// mining하면 to에게 코인을 보상으로 줍니다.
// 해당 트랜잭션을 Coinbase 라고 부르겠습니다.
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}

	// txin은 없다.
	txin := TxInput{[]byte{}, -1, data}
	// 100 coin이 to의 소유라는 내용의 txout
	txout := TxOutput{100, to}

	// in은 없는데 out만 존재하는 돈의 복사 현장이다...
	tx := Transaction{nil, []TxInput{txin}, []TxOutput{txout}}
	tx.SetID()

	return &tx
}

// 2 (blockchain.go 수정 후 참고)
// Transaction을 만드는 함수 입니다.
func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	// {from}이 {amount}를 지불하기 위해 필요한 {from}소유의 UTXO를 가지고 옵니다.
	// validOutputs : map[string][]int (txID => outIdx)
	acc, validOutputs := chain.FindSpendableOutputs(from, amount)

	// UTXO를 다 모았는데 amount보다 작다면 잔액 부족입니다.
	if acc < amount {
		log.Panic("Error: not enough funds")
	}

	// 모아온 UTXOs에 대해 for loop
	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			// {txID}를 가지는 트랜잭션의 {out}번째 {from}소유의 아웃풋이 인풋이 됩니다.
			input := TxInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	// 이제 트랜잭션의 인풋으로 사용될 UTXO를 모두 모았습니다.
	// 그리고 그 가치의 합은 acc가 될 것입니다.
	// 트랜잭션의 아웃풋은 전송될 TXO, 잔금(반환될) TXO
	// 항상 두개로 이루어집니다.

	// 전송될 TXO
	outputs = append(outputs, TxOutput{amount, to})

	// 반환될 TXO
	if acc > amount {
		outputs = append(outputs, TxOutput{acc - amount, from})
	}

	// 인풋과 아웃풋을 바탕으로 Transaction이 생성됩니다.
	tx := Transaction{nil, inputs, outputs}
	tx.SetID()

	return &tx
}

// 1
// 해당 트랜잭션이 Coinbase 인가?
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// 1
// Signature를 확인해서 같으면 풀 수 있는 (소유의) Input입니다.
func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// 1
// 공개키를 확인해서 같으면 풀 수 있는 (소유의) Input입니다.
func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
