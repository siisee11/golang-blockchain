package wallet

import (
	"log"

	"github.com/mr-tron/base58"
)

// Base64에서 6개의 문자를 제외한 Base58의 encoding
// 0 O l I + / 제외
func Base58Encode(input []byte) []byte {
	encode := base58.Encode(input)

	return []byte(encode)
}

// Base58의decoding
func Base58Decode(input []byte) []byte {
	decode, err := base58.Decode(string(input[:]))
	if err != nil {
		log.Panic(err)
	}

	return decode
}
