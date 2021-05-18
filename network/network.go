package network

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	mrand "math/rand"
	"os"
	"runtime"
	"sync"
	"syscall"

	"github.com/dgraph-io/badger"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/siisee11/golang-blockchain/blockchain"
	DEATH "github.com/vrecan/death/v3"
)

const (
	protocol      = "tcp" // 통신 프로토콜
	version       = 1     // version number
	commandLength = 12    // command string의 길이
)

var (
	Chain           *blockchain.BlockChain                    // Block의 정보를 담은 DB
	peers           *Peers                                    // Peer들의 정보를 담은 DB
	ha              host.Host                                 // 지금 노드의 host
	NodeId          string                                    // 지금 노드의 nodeId
	nodePeerId      string                                    // p2p에서 사용될 이 노드의 peerId
	minterAddress   string                                    // minter의 주소
	KnownPeers      = []string{}                              // 알려진 Peer들
	blocksInTransit = [][]byte{}                              // 전송 받아야할 블록들
	memoryPool      = make(map[string]blockchain.Transaction) // txID => Transaction
)

// print colour 참조용
const (
	InfoColor    = "\033[1;34m%s\033[0m"
	NoticeColor  = "\033[1;36m%s\033[0m"
	WarningColor = "\033[1;33m%s\033[0m"
	ErrorColor   = "\033[1;31m%s\033[0m"
	DebugColor   = "\033[0;36m%s\033[0m"
)

// 아래는 통신을 위한 구조들.
type Addr struct {
	AddrList []string
}

type Block struct {
	AddrFrom string
	Block    []byte
}

type GetBlocks struct {
	AddrFrom string
}

type GetData struct {
	AddrFrom string
	Type     string
	ID       []byte
}

// Inventory
type Inv struct {
	AddrFrom string
	Type     string
	Items    [][]byte
}

type Tx struct {
	AddrFrom    string
	Transaction []byte
}

type Version struct {
	Version    int
	BestHeight int
	AddrFrom   string
}

// Helper functionn
// network 통신을 위해 command를 byte 배열로 변환
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}
	return bytes[:]
}

// Helper functionn
// byte배열을 커맨드로 변환
func BytesToCmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

// KnownPeers들에게 블록을 달라고 요청
func RequestBlocks() {
	for _, node := range KnownPeers {
		SendGetBlocks(node)
	}
}

// {request}의 첫 commandLength byte는 커맨드
func ExtractCmd(request []byte) []byte {
	return request[:commandLength]
}

// KnownPeers에 자신의 address를 더해서 {addr}에게 addr 커맨드를 보냄
func SendAddr(addr string) {
	nodes := Addr{KnownPeers}
	nodes.AddrList = append(nodes.AddrList, nodePeerId)
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)

	SendData(addr, request)
}

// Block을 payload에 담아서 보냄
func SendBlock(addr string, b *blockchain.Block) {
	data := Block{nodePeerId, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)

	SendData(addr, request)
}

// {items}([]block의 해시 이나 tx)을 보냄
func SendInv(addr, kind string, items [][]byte) {
	inventory := Inv{nodePeerId, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)

	SendData(addr, request)
}

// Transaction을 보냄
func SendTx(addr string, tnx *blockchain.Transaction) {
	data := Tx{nodePeerId, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(addr, request)
}

// Transaction을 보냄 (전송 한번 후에 종료되는 경우)
func SendTxOnce(addr string, tnx *blockchain.Transaction) {
	data := Tx{nodePeerId, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendDataOnce(addr, request)
}

// Version을 보냄(Height, version)
func SendVersion(addr string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight()
	data := Version{version, bestHeight, nodePeerId}
	payload := GobEncode(data)
	request := append(CmdToBytes("version"), payload...)

	log.Printf("Send Version {version: %d, height: %d} to %s\n", version, bestHeight, addr)

	SendData(addr, request)
}

// Block들을 달라고 요청을 보냄
func SendGetBlocks(addr string) {
	payload := GobEncode(GetBlocks{nodePeerId})
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(addr, request)
}

// data를 달라고 요청을 보냄
func SendGetData(addr, kind string, id []byte) {
	payload := GobEncode(GetData{nodePeerId, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(addr, request)
}

// "addr" 커맨드를 처리함
func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	// request에서 앞 commandLength를 제외하면 payload
	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	// []byte => Addr
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// 받은 주소들을 KnownPeers에 추가합니다.
	KnownPeers = append(KnownPeers, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownPeers))

	// Block을 요청함.
	RequestBlocks()
}

// "block" 커맨드를 처리함.
func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	// 받아온 블록
	block := blockchain.Deserialize(blockData)

	chain.AddBlock(block)

	log.Printf("New block received. Add it(%x) to chain\n", block.Hash)

	if len(blocksInTransit) > 0 {
		// 아직 받아야하는 블록이 남아 있으면
		blockHash := blocksInTransit[0]
		// 다음 블록을 달라고 요청
		SendGetData(payload.AddrFrom, "block", blockHash)

		blocksInTransit = blocksInTransit[1:]
	} else {
		log.Println("All blocks received.")
		// 모든 블록을 다 받았다면 UTXO를 다시 인덱싱한다.
		UTXOset := blockchain.UTXOSet{Blockchain: chain}
		UTXOset.Reindex()
	}
}

// "getblocks" 커맨드를 처리함.
func HandleGetBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// Block의 모든 해시 값을 가져옵니다.
	blocks := chain.GetBlockHashes()
	log.Printf("Send %d block hashes to %s", len(blocks), payload.AddrFrom)
	SendInv(payload.AddrFrom, "block", blocks)
}

// "getdata" 커맨드를 처리함.
func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	// Type이 "block"이면 Block을 보내줌.
	if payload.Type == "block" {
		// payload.ID는 blockHash
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}

		// 요청한 노드에게 블록을 보냅니다.
		SendBlock(payload.AddrFrom, &block)
	}

	// Type이 "tx"이면 트랜잭션을 찾아서 보냄
	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTx(payload.AddrFrom, &tx)
	}

}

// "getversion" 커맨드를 처리함.
func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Got  Version {version: %d, height: %d} from %s\n", payload.Version, payload.BestHeight, payload.AddrFrom)

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	if bestHeight < otherHeight {
		log.Printf("Get blocks from peer %s", payload.AddrFrom)
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight {
		SendVersion(payload.AddrFrom, chain)
	}

	if !NodeIsKnown(payload.AddrFrom) {
		KnownPeers = append(KnownPeers, payload.AddrFrom)
	}
}

// "tx" 커맨드를 처리함.
// 트랜잭션을 받았을 떄 불리는 함수.
func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	txData := payload.Transaction
	// {tx} 받은 트랜잭션
	tx := blockchain.DeserializeTransaction(txData)
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	log.Printf("%s received Tx, now %d txs in memoryPool\n", nodePeerId, len(memoryPool))

	// KnownPeers 들에게 {tx}을 보낸다.
	for _, node := range KnownPeers {
		// 현재노드 {nodePeerId}가 아니고 {tx}를 전달받은 노드가 아니면
		if node != nodePeerId && node != payload.AddrFrom {
			// 받은 tx의 ID 전송
			SendInv(node, "tx", [][]byte{tx.ID})
		}
	}

	// memoryPool에 2개이상의 Tx가 있고 minterAddress가 존재하면(채굴 노드이면)
	if len(memoryPool) >= 2 && len(minterAddress) > 0 {
		MintTx(chain)
	}
}

// "inv" 커맨드를 처리함.
func HandleInv(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inv

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Received Inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		// 받아야하는 블록의 해시들. []blockhashes
		blocksInTransit = payload.Items

		// 첫번째 블록의 해시
		blockHash := payload.Items[0]
		// blockHash 값으로 데이터를 주라고 요청합니.
		SendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		// 받아야하는 블록해시 리스트에서 방금 getdata 요청을 보낸 블록해시를 제거합니다.
		for _, b := range blocksInTransit {
			if !bytes.Equal(b, blockHash) {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit = newInTransit
	}

	// txID를 받으면
	if payload.Type == "tx" {
		txID := payload.Items[0]

		// memoryPool에 해당 txID를 가진 트랜잭션이 저장되어 있지 않다면
		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			// txID로 트랜잭션을 가지고 온다.
			SendGetData(payload.AddrFrom, "tx", txID)
		}
	}
}

// Block을 채굴하여 트랜잭션을 기록 후 새로운 블록을 KnownPeers에게 알림.
func MintTx(chain *blockchain.BlockChain) {
	var txs []*blockchain.Transaction

	log.Println("Mint Transaction")

	// memoryPool에서 트랜잭션을 꺼내서 verify한 후 txs에 추가합니다.
	for id := range memoryPool {
		fmt.Printf("txID: %x\n", memoryPool[id].ID)
		tx := memoryPool[id]
		if chain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		}
	}

	if len(txs) == 0 {
		fmt.Println("All Transactions are invalid")
		return
	}

	// 채굴자 주소로 CoinbaseTx를 만들어 txs에 추가합니다.
	cbTx := blockchain.CoinbaseTx(minterAddress, "")
	txs = append(txs, cbTx)

	// {txs} 트랜잭션들을 인자로 Block을 생성합니다.
	newBlock := chain.MintBlock(txs)
	UTXOset := blockchain.UTXOSet{Blockchain: chain}
	UTXOset.Reindex()

	log.Printf("%s mint new block\n", minterAddress)

	// 새로운 블록에 포함된 트랜잭션을 memoryPool 에서 삭제합니다.
	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	// KnownPeers들에게 새로운 block을 전송합니다.
	for _, node := range KnownPeers {
		if node != nodePeerId {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	// 아직 memoryPool에 트랜잭션이 남았으면 다시 채굴.
	if len(memoryPool) > 0 {
		MintTx(chain)
	}
}

// P2P방식으로 request를 받으면 처리하는 로직
func HandleP2PConnection(rw *bufio.ReadWriter) {
	req, err := ioutil.ReadAll(rw)
	if err != nil {
		log.Panic(err)
	}

	command := BytesToCmd(req[:commandLength])

	switch command {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, Chain)
	case "inv":
		HandleInv(req, Chain)
	case "getblocks":
		HandleGetBlock(req, Chain)
	case "getdata":
		HandleGetData(req, Chain)
	case "tx":
		HandleTx(req, Chain)
	case "version":
		HandleVersion(req, Chain)
	default:
		fmt.Println("Unknown command")
	}
}

// Generic Encoding 함수
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

// {addr}가 KnownPeers에 속해있으면 true
func NodeIsKnown(addr string) bool {
	for _, node := range KnownPeers {
		if node == addr {
			return true
		}
	}
	return false
}

// 안전한 DB close
func CloseDB(db *badger.DB) {
	// SIGINT, SIGTERM : unix, linux / Interrupt : window
	d := DEATH.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		db.Close()
	})
}

// random peer ID를 가진 LibP2P 호스트를 만듭니다.
func makeBasicHost(listenPort int, secio bool, randseed int64) (host.Host, error) {
	// randseed가 0이면 완벽한 랜덤값이 아닙니다. 예측가능한 값이 사용되어 같은 priv가 생성될 것입니다.
	var r io.Reader
	if randseed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	// 이 호스트의 key pair를 만듭니다.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	// 옵션들.
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	}

	// 호스트를 만들어 리턴합니다.
	return libp2p.New(context.Background(), opts...)
}

// {data}(cmd + payload)를 보냄
// p2p 에서는 peer ID를 이용하여 통신합니다.
func SendData(destPeerID string, data []byte) {
	peerID, err := peer.Decode(destPeerID)
	if err != nil {
		log.Panic(err)
	}

	// {ha} => {peerID} 의 Stream을 만듭니다.
	// 이 Stream은 {peerID}호스트의 steamHandler에 의해 처리될 것입니다.
	s, err := ha.NewStream(context.Background(), peerID, "/p2p/1.0.0")
	if err != nil {
		log.Printf("%s is \033[1;33mnot reachable\033[0m\n", peerID)

		peers.DeletePeer(peerID)
		log.Printf("%s deleted\n", peerID)

		// TODO: 통신이 되지 않는 {peer}를 KnownPeers에서 삭제합니다.
		var updatedPeers []string

		// 통신이 되지 않는 {addr}를 KnownPeers에서 삭제합니다.
		for _, node := range KnownPeers {
			if node != destPeerID {
				updatedPeers = append(updatedPeers, node)
			}
		}

		KnownPeers = updatedPeers

		return
	}
	defer s.Close()

	_, err = s.Write(data)
	if err != nil {
		log.Println(err)
		return
	}
}

// {targetPeer}에게 {data}를 보냅니다.
// 1회성 host를 만들어 전송합니다.
func SendDataOnce(targetPeer string, data []byte) {
	host, err := libp2p.New(context.Background())
	if err != nil {
		log.Panic(err)
	}
	defer host.Close()
	ha = host

	destPeerID := addAddrToPeerstore(host, targetPeer)
	SendData(peer.Encode(destPeerID), data)
}

// 호스트의 0번째 주소를 알아옵니다.
func getHostAddress(_ha host.Host) string {
	// Build host multiaddress
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", _ha.ID().Pretty()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := _ha.Addrs()[0]
	return addr.Encapsulate(hostAddr).String()
}

// Stream을 받았을 때 처리하는 핸들러 함수
func handleStream(s network.Stream) {
	// 일이 다 끝나면 stream을 종료합니다.
	defer s.Close()

	// Non blocking read/write를 위해 버퍼 스트림을 만듭니다.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	// connection 처리는 asynchronous하게 go routine으로 처리
	go HandleP2PConnection(rw)
}

// DB에 저장된 Peer들에게 연락합니다.
func connectToKnownPeer(host host.Host, peers *Peers) bool {
	// 저장되어 있는 peer들을 출력합니다.
	peerAddrInfos := peers.FindAllAddrInfo()
	log.Println("\033[1;36mIn peers DB:\033[0m")
	for _, peerAddrInfo := range peerAddrInfos {
		fmt.Printf("%s\n", peerAddrInfo)
	}

	// 먼저 저장되어 있는 peer들에게 연결합니다.
	for _, peerinfo := range peerAddrInfos {
		// {host} => {peer} 의 Stream을 만듭니다.
		// 이 Stream은 {peer}호스트의 steamHandler에 의해 처리될 것입니다.
		s, err := host.NewStream(context.Background(), peerinfo.ID, "/p2p/1.0.0")
		if err != nil {
			log.Printf("%s is \033[1;33mnot reachable\033[0m\n", peerinfo.ID)

			// 연결할 수 없다면 peer DB에서 삭제합니다.
			peers.DeletePeer(peerinfo.ID)
			log.Printf("%s => %s deleted\n", peerinfo.ID, peerinfo.Addrs)

			// TODO: 통신이 되지 않는 {peer}를 KnownPeers에서 삭제합니다.
			var updatedPeers []string

			// 통신이 되지 않는 {addr}를 KnownPeers에서 삭제합니다.
			for _, node := range KnownPeers {
				if node != peer.Encode(peerinfo.ID) {
					updatedPeers = append(updatedPeers, node)
				}
			}

			KnownPeers = updatedPeers
		} else {
			// 연결되었으면 Version 메세지를 보낸다.
			SendVersion(peer.Encode(peerinfo.ID), Chain)
			s.Close()
			return true
		}
	}

	return false
}

// rendezvous point에서 다른 peer들의 정보를 알아와서 연결합니다.
func peerDiscovery(ctx context.Context, host host.Host, peers *Peers, rendezvous string, bootstrapPeers []ma.Multiaddr) {
	kademliaDHT, err := dht.New(ctx, host)
	if err != nil {
		panic(err)
	}

	log.Println("Bootstrapping the DHT")
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(err)
	}

	// Bootstrap 노드들은 네트워크에 속한 다른 노드들의 정보를 알려줍니다.
	// 물론 우리의 정보도 접속하는 다른 노드에게 전달합니다.
	var wg sync.WaitGroup
	for _, peerAddr := range bootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerinfo); err != nil {
				log.Fatalln(err)
			} else {
				log.Println("Connection established with bootstrap node:", *peerinfo)
			}
		}()
	}
	wg.Wait()

	// rendezvous point에 우리의 정보를 적습니다.
	log.Println("Announcing ourselves...")
	routingDiscovery := discovery.NewRoutingDiscovery(kademliaDHT)
	discovery.Advertise(ctx, routingDiscovery, rendezvous)
	log.Println("Successfully announced!")
	log.Println("Searching for other peers...")
	log.Printf("Now run \"go run main.go startp2p -rendezvous %s\" on a different terminal\n", rendezvous)

	// peer들을 찾습니다. []peer.AddrInfo를 리턴합니다.
	peerChan, err := routingDiscovery.FindPeers(ctx, rendezvous)
	if err != nil {
		panic(err)
	}

	for p := range peerChan {
		if p.ID == host.ID() {
			continue
		}

		// 유효한 Addrs를 가지고 있으면
		if len(p.Addrs) > 0 {
			log.Println("\033[1;36mConnecting to:\033[0m", p)
			// 이 정보를 Peer DB에 저장합니다
			peers.AddPeer(p)

			// Stream을 엽니다.
			s, err := ha.NewStream(context.Background(), p.ID, "/p2p/1.0.0")
			if err != nil {
				log.Printf("%s is \033[1;33mnot reachable\033[0m\n", p.ID)

				// Stream 생성에 에러가 생기면 PeerDB에서 Peer를 삭제합니다.
				peers.DeletePeer(p.ID)
				log.Printf("%s => %s \033[1;33mdeleted\033[0m\n", p.ID, p.Addrs)
			} else {
				s.Close()
				// {p}에게 {Chain}의 Version을 보냅니다.
				SendVersion(peer.Encode(p.ID), Chain)
			}

		} else {
			// 유효하지 않은 Peer입니다. 혹시 DB에 저장되어 있을 수 있으니 삭제합니다.
			peers.DeletePeer(p.ID)
			log.Println("\033[1;31mINVAILD ADDR\033[0m", p)
		}
	}
}

// Host를 시작합니다.
func StartHost(listenPort int, minter string, secio bool, randseed int64, rendezvous string, bootstrapPeers []ma.Multiaddr) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// minter의 주소를 global 변수에 저장.
	minterAddress = minter

	// {listenPort}가 nodeId로 쓰이게됩니다.
	NodeId = fmt.Sprintf("%d", listenPort)
	// chain을 전역변수에 저장
	Chain = blockchain.ContinueBlockChain(NodeId)
	go CloseDB(Chain.Database) // 하드웨어 인터럽트를 대기하고 있다가 안전하게 DB를 닫는 함수
	defer Chain.Database.Close()

	// P2P host를 만듭니다.
	host, err := makeBasicHost(listenPort, secio, randseed)
	if err != nil {
		log.Panic(err)
	}

	// {host}를 전역변수 {ha}에 저장합니다.
	ha = host
	// {nodePeerId}: 이 노드의 peer ID 입니다.
	// 통신에 Peer Id 가 사용됩니다.
	nodePeerId = peer.Encode(host.ID())

	if len(KnownPeers) == 0 {
		// KnownPeers[0]는 자기 자신입니다.
		KnownPeers = append(KnownPeers, nodePeerId)
	}

	fullAddr := getHostAddress(ha)
	log.Printf("I am %s\n", fullAddr)

	// StreamHandler를 시작합니다.
	ha.SetStreamHandler("/p2p/1.0.0", handleStream)

	// 저장되어 있는 peer들을 불러옵니다.
	peers, err := GetPeerDB(NodeId)
	if err != nil {
		log.Println(err)
	}
	go CloseDB(peers.Database)
	defer peers.Database.Close()

	// 저장되어 있는 피어에 우선 접속해봅니다.
	connected := connectToKnownPeer(host, peers)
	if !connected {
		// 저장되어 있는 피어와 연결되지 않았다면
		// 새로운 피어를 찾아서 연결합니다.
		peerDiscovery(ctx, host, peers, rendezvous, bootstrapPeers)
	}

	// Wait forever
	select {}
}

// peer의 {addr}를 받아 multiaddress로 파싱한 후 host의 peerstore에 저장합니다.
// 해당 정보로 peer ID를 알면 어떻게 통신해야하는 지 알 수 있습니다.
// peer의 ID를 반환합니다.
func addAddrToPeerstore(ha host.Host, addr string) peer.ID {
	// multiaddress로 파싱 후
	ipfsaddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		log.Fatalln(err)
	}

	// multiaddress에서 Address와 PeerID 정보를 알아옵니다.
	info, err := peer.AddrInfoFromP2pAddr(ipfsaddr)
	if err != nil {
		log.Fatalln(err)
	}

	// LibP2P가 참고할 수 있도록
	// Peer ID와 address를 peerstore에 저장합니다.
	ha.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
	return info.ID
}
