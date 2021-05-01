// tx.go
package blockchain

import (
	"bytes"

	"github.com/siisee11/golang-blockchain/wallet"
)

type TxOutput struct {
	Value int // 잔액

	// address를 decode하여 얻을 수 있는 값입니다.
	// 좀더 raw한 형태의 주소라고 생각하면됩니다.
	// 자세한 내용은 wallet문서를 참조하세요.
	PubKeyHash []byte
}

// Input으로 사용하고자 하는 UTXO를 가르킵니다.
type TxInput struct {
	ID        []byte // UTXO가 생성된 트랜잭션의 ID
	Out       int    // 그 트랜잭션에서 몇번째 UTXO였는 지
	Signature []byte // UTXO를 사용하려는 사람의 서명
	PubKey    []byte
}

// {value}와 {address}를 사용해 TXO를 만듭니다.
func NewTXOutput(value int, address string) *TxOutput {
	txo := &TxOutput{value, nil}
	txo.Lock([]byte(address))

	return txo
}

// Pubkey를 이용해 소유권 판별.
func (in *TxInput) UsesKey(pubKeyHash []byte) bool {
	lockingHash := wallet.PublicKeyHash(in.PubKey)

	return bytes.Compare(lockingHash, pubKeyHash) == 0
}

// {address}를 통해 pubKeyHash를 구해 TXO에 적습니다.
func (out *TxOutput) Lock(address []byte) {
	// Base58 Decode를 하고
	pubKeyHash := wallet.Base58Decode(address)
	// version byte와 checksum byte를 뺍니다.
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	out.PubKeyHash = pubKeyHash
}

// TXO의 pubKeyHash를 보고 소유권을 판단합니다.
func (out *TxOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	// 인자로 받은 pubKeyHash와 TXO의 pubKeyHash를 비교합니다.
	return bytes.Compare(out.PubKeyHash, pubKeyHash) == 0
}
