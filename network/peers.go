package network

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const peerFile = "./tmp/peers_%s.data"

type Peers struct {
	Peers map[peer.ID][]ma.Multiaddr
}

// Peers를 만듭니다.
func CreatePeers(nodeId string) (*Peers, error) {
	peers := Peers{}
	peers.Peers = make(map[peer.ID][]ma.Multiaddr)

	// 파일에 저장된 peers를 불러옵니다.
	err := peers.LoadFile(nodeId)

	return &peers, err
}

// Peers에 정보를 추가합니다.
func (pa *Peers) AddPeer(info peer.AddrInfo) {
	pa.Peers[info.ID] = info.Addrs
}

func (pa Peers) GetAddr(pid peer.ID) []ma.Multiaddr {
	return pa.Peers[pid]
}

// Peers에 저장된 모든 PeerId값을 반환합니다.
func (pa Peers) GetAllPeerIds() []peer.ID {
	var peerIds []peer.ID

	for peerId := range pa.Peers {
		peerIds = append(peerIds, peerId)
	}

	return peerIds
}

// 파일에 저장된 Peers를 읽어오는 함수
func (pa *Peers) LoadFile(nodeId string) error {
	peerFile := fmt.Sprintf(peerFile, nodeId)
	if _, err := os.Stat(peerFile); os.IsNotExist(err) {
		return err
	}

	var peeraddrs Peers

	fileConent, err := ioutil.ReadFile(peerFile)
	if err != nil {
		return err
	}

	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileConent))
	err = decoder.Decode(&peeraddrs)
	if err != nil {
		return err
	}

	pa.Peers = peeraddrs.Peers

	return nil
}

// Peers을 파일에 저장하는 함수
func (pa *Peers) SaveFile(nodeId string) {
	var content bytes.Buffer
	peerFile := fmt.Sprintf(peerFile, nodeId)

	gob.Register(elliptic.P256())

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(pa)
	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(peerFile, content.Bytes(), 0644)
	if err != nil {
		log.Panic(err)
	}
}
