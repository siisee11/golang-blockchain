package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"

	"github.com/siisee11/golang-blockchain/blockchain"
	DEATH "github.com/vrecan/death/v3"
)

// 	go get github.com/vrecan/death/v3

const (
	protocol      = "tcp" // 통신 프로토콜
	version       = 1     // version number
	commandLength = 12    // command string의 길이
)

var (
	nodeAddress   string // Server를 돌리는 바로 이 노드의 주소
	minterAddress string // minter의 주소
	// KnownNodes : 네트워크에 속한 알고있는 노드들
	// localhost:3000은 central node
	KnownNodes      = []string{"localhost:3000"}
	blocksInTransit = [][]byte{}
	memoryPool      = make(map[string]blockchain.Transaction) // txID => Transaction
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

// KnownNodes들에게 블록을 달라고 요청
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
}

// {request}의 첫 commandLength byte는 커맨드
func ExtractCmd(request []byte) []byte {
	return request[:commandLength]
}

// KnownNodes에 자신의 address를 더해서 {addr}에게 addr 커맨드를 보냄
func SendAddr(addr string) {
	nodes := Addr{KnownNodes}
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)

	SendData(addr, request)
}

// Block을 payload에 담아서 보냄
func SendBlock(addr string, b *blockchain.Block) {
	data := Block{nodeAddress, b.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)

	SendData(addr, request)
}

// {items}([]block의 해시 이나 tx)을 보냄
func SendInv(addr, kind string, items [][]byte) {
	inventory := Inv{nodeAddress, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)

	SendData(addr, request)
}

// Transaction을 보냄
func SendTx(addr string, tnx *blockchain.Transaction) {
	data := Tx{nodeAddress, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(addr, request)
}

// Version을 보냄(Height, version)
func SendVersion(addr string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight()
	data := Version{version, bestHeight, nodeAddress}
	payload := GobEncode(data)
	request := append(CmdToBytes("version"), payload...)

	log.Printf("Send Version {version: %d, height: %d} to %s\n", version, bestHeight, addr)

	SendData(addr, request)
}

// Block들을 달라고 요청을 보냄
func SendGetBlocks(addr string) {
	payload := GobEncode(GetBlocks{nodeAddress})
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(addr, request)
}

// data를 달라고 요청을 보냄
func SendGetData(addr, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(addr, request)
}

// request(cmd + payload)를 보냄
func SendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr)

	// {addr}에 연결이 안되면
	if err != nil {
		log.Printf("%s is not reachable\n", addr)
		var updatedNodes []string

		// 통신이 되지 않는 {addr}를 KnownNodes에서 삭제합니다.
		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}

		KnownNodes = updatedNodes

		return
	}

	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))
	if err != nil {
		log.Panic(err)
	}

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

	// 받은 주소들을 KnownNodes에 추가합니다.
	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("there are %d known nodes\n", len(KnownNodes))

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

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	log.Printf("Got  Version {version: %d, height: %d} from %s\n", payload.Version, payload.BestHeight, payload.AddrFrom)

	if bestHeight < otherHeight {
		log.Printf("Get blocks from peer %s", payload.AddrFrom)
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight {
		SendVersion(payload.AddrFrom, chain)
	}

	if !NodeIsKnown(payload.AddrFrom) {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
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

	log.Printf("%s received Tx, now %d txs in memoryPool\n", nodeAddress, len(memoryPool))

	// 중앙 노드이면
	if nodeAddress == KnownNodes[0] {
		// KnownNodes 들에게 {tx}을 보낸다.
		for _, node := range KnownNodes {
			// 현재노드 {nodeAddress}가 아니고 {tx}를 전달받은 노드가 아니면
			if node != nodeAddress && node != payload.AddrFrom {
				// 받은 tx의 ID 전송
				SendInv(node, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		// memoryPool에 2개이상의 Tx가 있고 minterAddress가 존재하면(채굴 노드이면)
		if len(memoryPool) >= 1 && len(minterAddress) > 0 {
			MintTx(chain)
		}
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

// Block을 채굴하여 트랜잭션을 기록 후 새로운 블록을 KnownNodes에게 알림.
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

	// KnownNodes들에게 새로운 block을 전송합니다.
	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	// 아직 memoryPool에 트랜잭션이 남았으면 다시 채굴.
	if len(memoryPool) > 0 {
		MintTx(chain)
	}
}

// request를 받으면 처리하는 로직
func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()

	if err != nil {
		log.Panic(err)
	}

	command := BytesToCmd(req[:commandLength])
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, chain)
	case "inv":
		HandleInv(req, chain)
	case "getblocks":
		HandleGetBlock(req, chain)
	case "getdata":
		HandleGetData(req, chain)
	case "tx":
		HandleTx(req, chain)
	case "version":
		HandleVersion(req, chain)
	default:
		fmt.Println("Unknown command")
	}
}

// Node(server)를 실행합니다.
// nodeID는 노드의 포트번호(NODE_ID), _minterAddress는 option
func StartServer(nodeID, _minterAddress string) {
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)
	// minter의 주소를 global 변수에 저장.
	minterAddress = _minterAddress
	// localhast:{nodeID} 주소에서 listen합니다.
	ln, err := net.Listen(protocol, nodeAddress)
	if err != nil {
		log.Panic(err)
	}
	defer ln.Close()

	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()
	go CloseDB(chain) // 하드웨어 인터럽트를 대기하고 있다가 안전하게 DB를 닫는 함수

	// Central node (NODE_ID==3000)이 아니면 Version을 보냄.
	if nodeAddress != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Panic(err)
		}
		// connection 처리는 asynchronous하게 go routine으로 처리
		go HandleConnection(conn, chain)

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

// {addr}가 KnownNodes에 속해있으면 true
func NodeIsKnown(addr string) bool {
	for _, node := range KnownNodes {
		if node == addr {
			return true
		}
	}
	return false
}

// 안전한 DB close
func CloseDB(chain *blockchain.BlockChain) {
	// SIGINT, SIGTERM : unix, linux / Interrupt : window
	d := DEATH.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
