// tx.go
package blockchain

// 트랜잭션의 인풋은 이전 트랜잭션에서의 아웃풋을 사용하는 것임을 기억해야합니다.
// {ID}를 가지는 트랜잭션의 {OUT}번째 {Sig} 소유의 아웃풋으로 생각할 수 있습니다.
type TxInput struct {
	ID  []byte
	Out int
	Sig string // 소유자의 서명
}

// 이것이 TXO(Transaction Output)입니다.
// "트랜잭션의 아웃풋"과 TXO라는 표현을 병행해서 사용합니다.
type TxOutput struct {
	Value int // 잔액

	// 소유자의 공개키
	// 여기서는 쉽게 소유자의 주소를 사용합니다.
	PubKey string
}

// Signature를 확인해서 같으면 풀 수 있는 (소유의) Input입니다.
func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// 공개키를 확인해서 같으면 풀 수 있는 (소유의) Input입니다.
func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
