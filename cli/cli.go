package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/siisee11/golang-blockchain/blockchain"
	"github.com/siisee11/golang-blockchain/network"
	"github.com/siisee11/golang-blockchain/wallet"
)

// CommandLine은 BlockChain과 상호작용을 해야합니다.
type CommandLine struct{}

// Cli help 메세지 입니다.
func (cli *CommandLine) printUsage() {
	fmt.Println("Usage: <optional>")
	fmt.Println(" getbalance -address ADDRESS - get the balance for address")
	fmt.Println(" createblockchain -address ADDRESS - creates a blockchain(miner: ADDRESS)")
	fmt.Println(" printchain - Prints the blocks in the chain")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT <-mint> - sends AMOUNT of coin from FROM to TO. Then -mint flag is set, mint off of this node")
	fmt.Println(" createwallet <-alias> - Creates a new Wallet (with ALIAS)")
	fmt.Println(" listaddresses - Lists the addresses in our wallet file")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
	fmt.Println(" startp2p <-minter ADDRESS> <-rendezvous STRING> <-peer MADDR> - Start a p2p host with ID specified in NODE_ID env var.")
}

// Args(arguments)가 1개면 명령어를 입력하지 않은 것이므로 종료합니다.
func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()

		// runtime.Goexit은 Go routine을 종료시키는 것이기 때문에
		// applicaion 강제 종료가 아니여서 DB가 정상 종료(close)될 수 있도록 해준다.
		runtime.Goexit()
	}
}

// {nodeId}를 listen 포트로 서버를 시작합니다.
// {minterAddress}가 있다면 이 서버는 minter로 동작하며
// transaction을 모은 후 블록을 생성하여 {minterAddress}에 보상을 받습니다.
// {dest}가 있다면 {dest}노드를 통해 p2p 네트워크에 접속합니다.
func (cli *CommandLine) StartP2P(nodeId, minterAddress string, secio bool, randseed int64, rendezvous string, bootstrapPeersString string) {
	fmt.Printf("Starting Host localhost:%s\n", nodeId)

	wallets, _ := wallet.CreateWallets(nodeId)
	minterAddress = wallets.GetAddress(minterAddress)

	if len(minterAddress) > 0 {
		if wallet.ValidateAddress(minterAddress) {
			fmt.Println("Mining is on. Address to receive rewards: ", minterAddress)
		} else {
			log.Panic("Wrong minter address!")
		}
	}

	port, err := strconv.Atoi(nodeId)
	if err != nil {
		log.Panic(err)
	}

	var bootstrapPeers []maddr.Multiaddr

	if bootstrapPeersString == "" {
		bootstrapPeers = dht.DefaultBootstrapPeers
	}

	network.StartHost(port, minterAddress, secio, 0, rendezvous, bootstrapPeers)
}

// UTXOSet을 rebuild합니다.
func (cli *CommandLine) reindexUTXO(nodeId string) {
	chain := blockchain.ContinueBlockChain(nodeId)
	defer chain.Database.Close()

	UTXOset := blockchain.UTXOSet{Blockchain: chain}
	UTXOset.Reindex()

	count := UTXOset.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

// Wallet을 생성합니다.
func (cli *CommandLine) createWallet(nodeId, alias string) {
	wallets, _ := wallet.CreateWallets(nodeId)
	address := wallets.AddWallet(alias)
	wallets.SaveFile(nodeId)

	fmt.Printf("New address is: %s   (%s)\n", address, alias)
}

// Wallets에 저장된 Wallet의 address를 출력합니다.
func (cli *CommandLine) listAddresses(nodeId string) {
	wallets, _ := wallet.CreateWallets(nodeId)
	aliases := wallets.GetAllAliases()

	for _, alias := range aliases {
		fmt.Printf("%s => %s\n", alias, wallets.GetAddress(alias))
	}
}

// Chain을 순회하며 블록을 출력합니다.
func (cli *CommandLine) printChain(nodeId string) {
	chain := blockchain.ContinueBlockChain(nodeId) // blockchain을 DB로 부터 받아온다.
	defer chain.Database.Close()
	iter := chain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		for _, tx := range block.Transactions {
			fmt.Println(tx)
			if !tx.IsCoinbase() {
				fmt.Printf("Transcation verification: %s\n", strconv.FormatBool(chain.VerifyTransaction(tx)))
			}
		}
		fmt.Println()

		// if Genesis
		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) createBlockChain(alias, nodeId string) {
	wallets, _ := wallet.CreateWallets(nodeId)
	address := wallets.GetAddress(alias)
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not Valid")
	}
	chain := blockchain.InitBlockChain(address, nodeId)
	defer chain.Database.Close()

	UTXOset := blockchain.UTXOSet{Blockchain: chain}
	UTXOset.Reindex()

	fmt.Println("Finished!")
}

func (cli *CommandLine) getBalance(alias, nodeId string) {
	wallets, _ := wallet.CreateWallets(nodeId)
	address := wallets.GetAddress(alias)
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not Valid")
	}
	chain := blockchain.ContinueBlockChain(nodeId) // blockchain을 DB로 부터 받아온다.
	UTXOset := blockchain.UTXOSet{Blockchain: chain}
	defer chain.Database.Close()

	balance := 0
	// Human readable Address를 PubKeyHash로 다시 변환.
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	UTXOs := UTXOset.FindUnspentTransactions(pubKeyHash)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

// {from}에서 {to}로 {amount}만큼 보냅니다.
// {mintNow}가 true이면 send트랜잭션을 담은 블록을 생성하고
// {mintNow}가 false이면 트랜잭션을 만들어 중앙 노드(targetPeer)에게 보냅니다.
func (cli *CommandLine) send(alias, to, targetPeer string, amount int, nodeId string, mintNow bool) {
	wallets, _ := wallet.CreateWallets(nodeId)
	from := wallets.GetAddress(alias)
	if !wallet.ValidateAddress(from) {
		log.Panic("Address is not Valid")
	}
	if !wallet.ValidateAddress(to) {
		log.Panic("Address is not Valid")
	}
	chain := blockchain.ContinueBlockChain(nodeId) // blockchain을 DB로 부터 받아온다.
	UTXOset := blockchain.UTXOSet{Blockchain: chain}
	defer chain.Database.Close()

	wallets, err := wallet.CreateWallets(nodeId)
	if err != nil {
		log.Panic(err)
	}
	wallet := wallets.GetWallet(from)

	tx := blockchain.NewTransaction(&wallet, to, amount, &UTXOset) // send 트랜잭션도 생성하여
	if mintNow {
		cbTx := blockchain.CoinbaseTx(from, "") // 코인베이스 트랜잭션을 생성하고
		txs := []*blockchain.Transaction{cbTx, tx}
		block := chain.MintBlock(txs)
		UTXOset.Update(block)
	} else {
		network.SendTxOnce(targetPeer, tx)
		fmt.Println("send tx")
	}

	fmt.Println("Success!")
}

func (cli *CommandLine) Run() {
	cli.validateArgs()

	// NODE_ID라는 환경변수를 읽어서 지역 변수에 저장합니다.
	nodeId := os.Getenv("NODE_ID")
	if nodeId == "" {
		fmt.Println("NODE_ID env is not set! $ export NODE_ID=<id>")
		runtime.Goexit()
	}

	// Go의 option 처리하는 함수들.
	startP2PCmd := flag.NewFlagSet("startp2p", flag.ExitOnError)
	reIndexUtxoCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)
	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	createWalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listAddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)

	bootstrapPeersString := startP2PCmd.String("peer", "", "Adds a peer multiaddress to the bootstrap list")
	rendezvous := startP2PCmd.String("rendezvous", "jy blockchain", "Unique string to identify group of nodes. Share this with your friends to let them connect with you")
	secio := startP2PCmd.Bool("secio", false, "P2P network security I/O")
	startP2PMinter := startP2PCmd.String("minter", "", "Enable minting mode and send reward to minter")
	getBalanceAddress := getBalanceCmd.String("address", "", "The address")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "Miner address")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Dest wallet address")
	peerId := sendCmd.String("peer", "", "Target Peer Id")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")
	sendMint := sendCmd.Bool("mint", false, "Mine immediately on the same node")
	createWalletAlias := createWalletCmd.String("alias", "", "Name wallet")

	switch os.Args[1] {
	case "startp2p":
		err := startP2PCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "reindexutxo":
		err := reIndexUtxoCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "getbalance":
		err := getBalanceCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "createblockchain":
		err := createBlockchainCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "send":
		err := sendCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "createwallet":
		err := createWalletCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "listaddresses":
		err := listAddressesCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}

	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if startP2PCmd.Parsed() {
		cli.StartP2P(nodeId, *startP2PMinter, *secio, 0, *rendezvous, *bootstrapPeersString)
	}

	if reIndexUtxoCmd.Parsed() {
		cli.reindexUTXO(nodeId)
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress, nodeId)
	}

	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockchainAddress, nodeId)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount == 0 || (!*sendMint && *peerId == "") {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *peerId, *sendAmount, nodeId, *sendMint)
	}

	if printChainCmd.Parsed() {
		cli.printChain(nodeId)
	}

	if createWalletCmd.Parsed() {
		cli.createWallet(nodeId, *createWalletAlias)
	}

	if listAddressesCmd.Parsed() {
		cli.listAddresses(nodeId)
	}
}
