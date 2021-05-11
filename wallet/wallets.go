// wallet/wallets.go
package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

// wallet을 저장할 파일의 이름
const walletFile = "./tmp/wallets_%s.data"

// Wallets는 Wallet들의 매핑을 가진다.
type Wallets struct {
	Wallets map[string]*Wallet
}

// Wallets를 만듭니다.
func CreateWallets(nodeId string) (*Wallets, error) {
	wallets := Wallets{}
	wallets.Wallets = make(map[string]*Wallet)

	// 파일에 저장된 wallets를 불러옵니다.
	err := wallets.LoadFile(nodeId)

	return &wallets, err
}

// Wallets에 Wallet을 추가합니다.
func (ws *Wallets) AddWallet() string {
	// wallet을 만들고
	wallet := MakeWallet()
	// wallet의 주소를 string형태로 저장합니다.
	address := fmt.Sprintf("%s", wallet.Address())

	// address => wallet 을 매핑에 넣습니다.
	ws.Wallets[address] = wallet

	return address
}

// Wallets에 저장된 모든 address값을 반환합니다.
func (ws Wallets) GetAllAddresses() []string {
	var addresses []string

	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}

	return addresses
}

// address에 해당하는 wallet을 반환합니다.
func (ws Wallets) GetWallet(address string) Wallet {
	return *ws.Wallets[address]
}

// 파일에 저장된 Wallets를 읽어오는 함수
func (ws *Wallets) LoadFile(nodeId string) error {
	walletFile := fmt.Sprintf(walletFile, nodeId)
	if _, err := os.Stat(walletFile); os.IsNotExist(err) {
		return err
	}

	var wallets Wallets

	fileConent, err := ioutil.ReadFile(walletFile)
	if err != nil {
		return err
	}

	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileConent))
	err = decoder.Decode(&wallets)

	if err != nil {
		return err
	}

	ws.Wallets = wallets.Wallets

	return nil
}

// Wallets을 파일에 저장하는 함수
func (ws *Wallets) SaveFile(nodeId string) {
	var content bytes.Buffer
	walletFile := fmt.Sprintf(walletFile, nodeId)

	gob.Register(elliptic.P256())

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)
	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(walletFile, content.Bytes(), 0644)
	if err != nil {
		log.Panic(err)
	}
}
