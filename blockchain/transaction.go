package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/siisee11/golang-blockchain/wallet"
)

// Transaction은 Input, Output으로 구성되어있습니다.
type Transaction struct {
	ID      []byte
	Inputs  []TxInput
	Outputs []TxOutput
}

// Transaction structure를 []byte로 인코딩하는 함수
func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

// Serialized Transaction을 이용해서 Hash값 계산
func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

// mining하면 to에게 코인을 보상으로 주는 Coinbase Transaction.
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}

	txin := TxInput{[]byte{}, -1, nil, []byte(data)}
	txout := NewTXOutput(100, to)

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}}
	tx.ID = tx.Hash()

	return &tx
}

// Transaction을 만드는 함수 입니다.
func NewTransaction(from, to string, amount int, chain *BlockChain) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	// Wallets에서 {from}의 address갖는 wallet을 가져옵니다.
	wallets, err := wallet.CreateWallets()
	Handle(err)
	w := wallets.GetWallet(from)
	// Public key로 부터 publicKeyHash를 생성합니다.
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	// {from}이 {amount}를 지불하기 위해 필요한 {from}소유의 UTXO를 가지고 옵니다.
	// {from}의 공개키 해시 {pubKeyHash}를 이용합니다.
	acc, validOutputs := chain.FindSpendableOutputs(pubKeyHash, amount)

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
			// 4번째 인자로 {from}의 PublicKey가 추가됩니다.
			input := TxInput{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	// 이제 트랜잭션의 인풋으로 사용될 UTXO를 모두 모았습니다.
	// 그리고 그 가치의 합은 acc가 될 것입니다.
	// 트랜잭션의 아웃풋은 전송될 TXO, 잔금(반환될) TXO
	// 항상 두개로 이루어집니다.

	// 전송될 TXO을 만듭니다.
	outputs = append(outputs, *NewTXOutput(amount, to))

	// 반환될 TXO
	if acc > amount {
		outputs = append(outputs, TxOutput{acc - amount, pubKeyHash})
	}

	// 인풋과 아웃풋을 바탕으로 Transaction이 생성됩니다.
	tx := Transaction{nil, inputs, outputs}
	tx.ID = tx.Hash()
	chain.SignTransaction(&tx, w.PrivateKey)

	return &tx
}

// 해당 트랜잭션이 Coinbase 인가?
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

// 소비하려는 UTXO(UTXO-IN)가 자신의 소유임을 Sign하는 과정
// 이는 Verity함수에서 Validator에 의해 검증된다.
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {
	if tx.IsCoinbase() {
		return
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction is not corret")
		}
	}

	// Signature와 PubKey field가 비어있는 트랜잭션을 복사해 만든다.
	txCopy := tx.TrimmedCopy()

	// 트랜잭션의 각 인풋에 서명함.
	// 각 인풋에 대해 아래 과정을 거침.
	// 1. 인풋의 이전 트랜잭션을 확인하여 PubKeyHash를 가져와 저장.
	// 2. 트랜잭션과 private key를 이용해 signature를 구함.
	// 3. Signature를 트랜잭션에 추가함.
	for inId, in := range txCopy.Inputs {
		prevTX := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		txCopy.Inputs[inId].PubKey = prevTX.Outputs[in.Out].PubKeyHash

		dataToSign := fmt.Sprintf("%x\n", txCopy)

		r, s, err := ecdsa.Sign(rand.Reader, &privKey, []byte(dataToSign))
		Handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inId].Signature = signature
		txCopy.Inputs[inId].PubKey = nil
	}

	// 모든 UTXO-IN에 서명을 완료 후 종료.
}

// 해당 트랜잭션이 유효한 트랜잭션인지 확인하는 함수.
func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction is not corret")
		}
	}

	// Signature와 PubKey field가 비어있는 트랜잭션을 복사해 만든다.
	txCopy := tx.TrimmedCopy()
	// P-256(secp256r1) elliptic curve를 사용. (비트코인에서는 secp256k1을 사용한다.)
	curve := elliptic.P256()

	// Public key로 각 Input에 포함되어있는 Signature가 유효한지 판별.
	for inId, in := range tx.Inputs {
		// Sign 할때와 같은 Hash값을 얻기 위해서 똑같은 과정을 거친다.
		prevTx := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash

		// signature는 S값과 R값으로 나뉘어진다.
		r := big.Int{}
		s := big.Int{}
		sigLen := len(in.Signature)
		r.SetBytes(in.Signature[:(sigLen / 2)])
		s.SetBytes(in.Signature[(sigLen / 2):])

		x := big.Int{}
		y := big.Int{}
		KeyLen := len(in.PubKey)

		x.SetBytes(in.PubKey[:(KeyLen / 2)])
		y.SetBytes(in.PubKey[(KeyLen / 2):])

		dataToVerify := fmt.Sprintf("%x\n", txCopy)

		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}
		// Public Key와 트랜잭션의 해시값, Signature(R,S)를 가지고 유효성을 판별한다.
		if ecdsa.Verify(&rawPubKey, []byte(dataToVerify), &r, &s) == false {
			return false
		}
		txCopy.Inputs[inId].PubKey = nil
	}

	// 모든 인풋(UTXO-IN)에 대해 검사를 통과하였으면 검증 성공
	return true
}

// TxInput의 pubkey와 signature를 nil로 초기화하며 복사
func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	for _, in := range tx.Inputs {
		inputs = append(inputs, TxInput{in.ID, in.Out, nil, nil})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TxOutput{out.Value, out.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs}

	return txCopy
}

// Transaction information을 출력할 때 사용하는 함수.
func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("+- Transaction %x", tx.ID))
	lines = append(lines, fmt.Sprintf("|"))
	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("+--- Input %d:", i))
		lines = append(lines, fmt.Sprintf("|  +-- TXID: 		%x", input.ID))
		lines = append(lines, fmt.Sprintf("|  +-- Out: 		%d", input.Out))
		lines = append(lines, fmt.Sprintf("|  +-- Signature: 	%x", input.Signature))
		lines = append(lines, fmt.Sprintf("|  +-- PubKey: 		%x", input.PubKey))
	}

	for i, output := range tx.Outputs {
		if i == len(tx.Outputs)-1 {
			lines = append(lines, fmt.Sprintf("|"))
			lines = append(lines, fmt.Sprintf("+--- Output %d:", i))
			lines = append(lines, fmt.Sprintf("   +- Value: 		%d", output.Value))
			lines = append(lines, fmt.Sprintf("   +- Script:		%x", output.PubKeyHash))
			break
		}
		lines = append(lines, fmt.Sprintf("|"))
		lines = append(lines, fmt.Sprintf("+--- Output %d:", i))
		lines = append(lines, fmt.Sprintf("|  +- Value: 		%d", output.Value))
		lines = append(lines, fmt.Sprintf("|  +- Script:		%x", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
