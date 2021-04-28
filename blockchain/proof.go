package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/big"
)

// 우리가 찾고자하는 정답을 정의하겠습니다.
// 우리는 256bit중 왼쪽 {Difficulty}만큼의 bit가 0인 답을 원합니다.
const Difficulty = 18

type ProofOfWork struct {
	Block  *Block
	Target *big.Int // 문제의 답 (Difficulty로 부터 얻어낸다.)

	// big.Int에 관한 내용 참고 https://golang.org/pkg/math/big/
}

// Block을 받아서 ProofOfWork struct를 반환한다.
// Difficulty로 Target을 만든다.
func NewProof(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	// 1을 왼쪽으로 256-Difficulty 만큼 이동시킨다.
	// PoW에서 target보다 작은 수가 나오는 것을 정답으로 할 것이다.
	// ** 작다는 뜻을 잘 생각해보자 왼쪽에 0이 Difficulty만큼 나오는 것과 같은 뜻이다.
	target.Lsh(target, uint(256-Difficulty))

	pow := &ProofOfWork{b, target}

	return pow
}

// Block의 데이터, 이전 해시, nonce, Difficulty 값을 모두 합쳐서 데이터를 만든다.
// 이 데이터를 sha256한 값이 정답이라면 이 데이터가 적힌 블록이 추가된다.
func (pow *ProofOfWork) InitData(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.Block.PrevHash,
			pow.Block.HashTransactions(), // Transaction들의 해시
			ToHex(int64(nonce)),
			ToHex(int64(Difficulty)),
		},
		[]byte{}, // seperator
	)
	return data
}

// PoW를 계산하여 nonce와 정답 hash값을 반환하는 함수
func (pow *ProofOfWork) Run() (int, []byte) {
	var intHash big.Int
	var hash [32]byte
	nonce := 0

	// 사실상 무한루프
	for nonce < math.MaxInt64 {
		// nonce를 포함하여 계산된 데이터를 가져온다.
		data := pow.InitData(nonce)
		// 데이터의 해시값.
		hash = sha256.Sum256(data)

		fmt.Printf("\r%x", hash)
		// 해시값으로 big.Int만듬
		intHash.SetBytes(hash[:])

		if intHash.Cmp(pow.Target) == -1 {
			// Target보다 intHash가 작다는 뜻, 즉 정답.
			break
		} else {
			// 다음 논스를 시도하자
			nonce++
		}
	}
	fmt.Println()

	// nonce와 결과 hash값을 반환
	return nonce, hash[:]
}

// 정답이 맞는지 검사하는 과정이다.
// Run에 비해 얼마나 쉬운지 알 수 있다. (단방향성)
func (pow *ProofOfWork) Validate() bool {
	var intHash big.Int
	// 블록에 포함된 Nonce를 통해 데이터 재현
	data := pow.InitData(pow.Block.Nonce)
	hash := sha256.Sum256(data)
	intHash.SetBytes(hash[:])

	return intHash.Cmp(pow.Target) == -1
}

// int64를 받아서 바이트로 변환하는 유틸리티 함수
func ToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}
