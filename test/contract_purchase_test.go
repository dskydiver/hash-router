package contractmanager

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/TitanInd/hashrouter/blockchain"
	"gitlab.com/TitanInd/hashrouter/lib"
	"gitlab.com/TitanInd/hashrouter/lumerinlib/clonefactory"
)

var sellerAddress = common.HexToAddress("0xC3AcdAE18291bFEB0671d1caAb1d13Fe04164f75")
var sellerPrivateKey = "24c1613e07ac889f90c08f728f281e61f5e6b95d3d69c307513599f463c7d237"
var buyerAddress = common.HexToAddress("0x0FDcC9fF7D6F5f79c4e80e797916713a2d05A9cA")
var buyerPrivateKey = "5a25d76802639b7df2f8b9c0339e67662db8a5e81368288dde5bcef0bf606de9"
var gethNodeAddress = "wss://ropsten.infura.io/ws/v3/4b68229d56fe496e899f07c3d41cb08a"
var clonefactoryAddress common.Address = common.HexToAddress("0xe91be01493f4ae28297790277303926aaec604dc")
var hashrateContractAddress common.Address = common.HexToAddress("0x3b6fE2c6AcD5B52a703a9653f4af44B1176978f4")

// 0x4b6cc541CB35F21323077a84EDE6A662155a0A83 0x4b5C5b20B19B301A6c28cD5060114176Cfc191D5 0x9f8a67886345fd46D3163634b57BEC47D8BB2875 0xaA1A80580B5a9586Cd6dfc24D8e94c1E57308d4c 0x3b6fE2c6AcD5B52a703a9653f4af44B1176978f4
var poolUrl = "stratum+tcp://rbajollari.contract5:@stratum.slushpool.com:3333"

func TestHashrateContractCreation(t *testing.T) {
	// hashrate contract params
	price := 0
	limit := 0
	speed := 10000000000000
	length := 1800

	log, _ := lib.NewLogger(false)
	client, err := blockchain.NewEthClient(gethNodeAddress, log)
	if err != nil {
		t.Fatal(err)
	}

	CreateHashrateContract(client, sellerAddress, sellerPrivateKey, clonefactoryAddress, price, limit, speed, length, clonefactoryAddress)

	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{
		Addresses: []common.Address{clonefactoryAddress},
	}, logs)
	if err != nil {
		t.Fatal(err)
	}

	for {
		select {
		case err := <-sub.Err():
			t.Fatalf("Error::%v", err)
		case event := <-logs:
			if event.Topics[0].Hex() == blockchain.ContractCreatedHex {
				hashrateContractAddress := common.HexToAddress(event.Topics[1].Hex())
				fmt.Printf("Address of created Hashrate Contract: %v\n\n", hashrateContractAddress.Hex())
			}
		}
	}
}

func TestHashrateContractPurchase(t *testing.T) {

	log, _ := lib.NewLogger(false)
	client, err := blockchain.NewEthClient(gethNodeAddress, log)
	if err != nil {
		t.Fatal(err)
	}

	PurchaseHashrateContract(client, buyerAddress, buyerPrivateKey, clonefactoryAddress, hashrateContractAddress, buyerAddress, poolUrl)

	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{
		Addresses: []common.Address{clonefactoryAddress},
	}, logs)
	if err != nil {
		t.Fatal(err)
	}

	for {
		select {
		case err := <-sub.Err():
			t.Fatalf("Error::%v", err)
		case event := <-logs:

			if event.Topics[0].Hex() == blockchain.ContractPurchasedHex {
				hashrateContractAddress := common.HexToAddress(event.Topics[1].Hex())
				fmt.Printf("Address of purchased Hashrate Contract: %v\n\n", hashrateContractAddress.Hex())
			}
		}
	}
}

func CreateHashrateContract(client *ethclient.Client,
	fromAddress common.Address,
	privateKeyString string,
	contractAddress common.Address,
	_price int,
	_limit int,
	_speed int,
	_length int,
	_validator common.Address) error {
	privateKey, err := crypto.HexToECDSA(privateKeyString)
	if err != nil {
		return err
	}

	time.Sleep(time.Millisecond * 700)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return err
	}
	fmt.Println("Nonce: ", nonce)

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return err
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)      // in wei
	auth.GasLimit = uint64(6000000) // in units
	auth.GasPrice = gasPrice

	instance, err := clonefactory.NewClonefactory(contractAddress, client)
	if err != nil {
		return err
	}

	price := big.NewInt(int64(_price))
	limit := big.NewInt(int64(_limit))
	speed := big.NewInt(int64(_speed))
	length := big.NewInt(int64(_length))
	tx, err := instance.SetCreateNewRentalContract(auth, price, limit, speed, length, _validator, "")
	if err != nil {
		return err
	}

	fmt.Printf("tx sent: %s\n", tx.Hash().Hex())
	return nil
}

func PurchaseHashrateContract(client *ethclient.Client,
	fromAddress common.Address,
	privateKeyString string,
	contractAddress common.Address,
	_hashrateContract common.Address,
	_buyer common.Address,
	poolData string) error {
	privateKey, err := crypto.HexToECDSA(privateKeyString)
	if err != nil {
		return err
	}

	time.Sleep(time.Millisecond * 700)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return err
	}
	fmt.Println("Nonce: ", nonce)

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return err
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)      // in wei
	auth.GasLimit = uint64(6000000) // in units
	auth.GasPrice = gasPrice

	instance, err := clonefactory.NewClonefactory(contractAddress, client)
	if err != nil {
		return err
	}

	tx, err := instance.SetPurchaseRentalContract(auth, _hashrateContract, poolData)
	if err != nil {
		return err
	}
	fmt.Printf("tx sent: %s\n\n", tx.Hash().Hex())
	fmt.Printf("Hashrate Contract %s, was purchased by %s\n\n", _hashrateContract, _buyer)
	return nil
}