package network

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	peerDBPath = "./tmp/peers_%s"
	peerDBFile = "MANIFEST"
)

type Peers struct {
	//	Peers map[peer.ID][]ma.Multiaddr
	Database *badger.DB
}

func PeerDBexist(path string) bool {
	if _, err := os.Stat(path + "/" + peerDBFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Peers를 만듭니다.
func GetPeerDB(nodeId string) (*Peers, error) {
	path := fmt.Sprintf(peerDBPath, nodeId)

	// File명을 통해 DB를 엽니다.
	opts := badger.DefaultOptions(path)
	// log 무시
	//	opts.Logger = nil
	db, err := openDB(path, opts)
	if err != nil {
		log.Panic(err)
	}

	peers := Peers{db}

	return &peers, err
}

// Peers에 정보를 추가합니다.
// []byte(peer.ID) => peer.AddrInfo
func (pa *Peers) AddPeer(info peer.AddrInfo) {
	err := pa.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(info.ID)); err == nil {
			return nil
		}

		infoData, _ := info.MarshalJSON()
		err := txn.Set([]byte(info.ID), infoData)

		return err
	})
	if err != nil {
		log.Panic(err)
	}
}

// Peers에서 {pid}정보를 삭제합니다..
func (pa *Peers) DeletePeer(pid peer.ID) {
	fmt.Println(peer.Encode(pid))
	err := pa.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Delete([]byte(pid)); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

func (pa Peers) FindAllAddrInfo() []peer.AddrInfo {
	db := pa.Database
	var addrInfos []peer.AddrInfo

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				fmt.Printf("key=%s, value=%s\n", k, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}
	return addrInfos
}

func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}

func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
}
