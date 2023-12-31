package blockchain

import (
	"context"
	"encoding/hex"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/interop"
	"gitlab.com/TitanInd/hashrouter/lib"
	"gitlab.com/TitanInd/hashrouter/lumerinlib/clonefactory"
	"gitlab.com/TitanInd/hashrouter/lumerinlib/implementation"
)

type closeout struct {
	fromAddress     string
	contractAddress string
	closeoutType    int64
}

type EthereumGateway struct {
	client                 EthereumClient
	cloneFactory           *clonefactory.Clonefactory
	sellerPrivateKeyString string
	cloneFactoryAddr       common.Address
	log                    interfaces.ILogger

	startCloseout chan *closeout
	endCloseout   chan error
	retry         lib.RetryFn
}

func NewEthereumGateway(ethClient EthereumClient, privateKeyString string, cloneFactoryAddrStr string, log interfaces.ILogger, retry lib.RetryFn) (*EthereumGateway, error) {
	// TODO: extract it to dependency injection, because we'll going to have only one cloneFactory per project
	cloneFactoryAddr := common.HexToAddress(cloneFactoryAddrStr)
	cloneFactory, err := clonefactory.NewClonefactory(cloneFactoryAddr, ethClient)
	if err != nil {
		return nil, err
	}

	g := &EthereumGateway{
		client:                 ethClient,
		sellerPrivateKeyString: privateKeyString,
		cloneFactoryAddr:       common.HexToAddress(cloneFactoryAddrStr),
		cloneFactory:           cloneFactory,
		log:                    log,
		startCloseout:          make(chan *closeout),
		endCloseout:            make(chan error),
		retry:                  retry,
	}

	go func() {
		for {
			closeout := <-g.startCloseout
			g.endCloseout <- g.setContractCloseOut(closeout.fromAddress, closeout.contractAddress, closeout.closeoutType)
		}
	}()

	return g, nil
}

// SubscribeToCloneFactoryEvents returns channel with events like new contract creation and contract purchasing
func (g *EthereumGateway) SubscribeToCloneFactoryEvents(ctx context.Context) (chan types.Log, interop.BlockchainEventSubscription, error) {
	return g.SubscribeToContractEvents(ctx, g.cloneFactoryAddr)
}

// SubscribeToContractEvents returns channel with events for particular contract
func (g *EthereumGateway) SubscribeToContractEvents(ctx context.Context, contractAddress common.Address) (chan types.Log, ethereum.Subscription, error) {
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddress},
	}

	// timeoutCtx, _ := context.WithTimeout(ctx, g.subscribeTimeout)
	// defer cancelFunc()

	logs, sub, err := g.subscribeFilterLogsReconnect(ctx, query, g.retry)
	if err != nil {
		g.log.Error(err)
		return logs, sub, err
	}

	return logs, sub, nil
}

func (g *EthereumGateway) subscribeFilterLogsReconnect(ctx context.Context, query ethereum.FilterQuery, retryFn lib.RetryFn) (chan types.Log, ethereum.Subscription, error) {
	var sub ethereum.Subscription
	logs := make(chan types.Log)
	attempt := 0

	go func() {
		for {
			g.log.Debugf("log subscription init")

			// temporary channel to be used for each attempt to subscribe
			channel := make(chan types.Log)

			sub, err := g.client.SubscribeFilterLogs(ctx, query, channel)
			if err != nil {
				attempt++
				if retryFn != nil {
					duration, done := retryFn(attempt)
					if done {
						g.log.Error("retries exhaused")
						return
					}
					g.log.Warnf("sleeping %.1f s attempt %d", duration.Seconds(), attempt)
					time.Sleep(duration)
				}
				g.log.Errorf("error during log subscription init, reconnection... %s", err)
				continue
			}
			attempt = 0

		MSG_LOOP:
			for {
				select {
				case msg := <-channel:
					logs <- msg
				case err := <-sub.Err():
					if err != nil {
						g.log.Errorf("error during log subscription read %s", err)
						break MSG_LOOP
					}
					// if err==nil then unsubscribe was called
					close(logs)
					return
				case <-ctx.Done():
					g.log.Warnf("context closed in eth gateway %s", ctx.Err())
					close(logs)
					return
				}
			}
		}
	}()

	errCh := make(chan error, 1)
	wrappedSub := NewClientSubscription(errCh, func() {
		if sub != nil {
			sub.Unsubscribe()
		}
	})

	return logs, wrappedSub, nil
}

// ReadContract reads contract information encoded in the blockchain
func (g *EthereumGateway) ReadContract(contractAddress common.Address) (interface{}, error) {
	var contractData ContractData
	instance, err := implementation.NewImplementation(contractAddress, g.client)
	if err != nil {
		g.log.Error(err)
		return contractData, err
	}

	url, err := instance.EncryptedPoolData(nil)
	if err != nil {
		g.log.Error(err)
		return contractData, err
	}

	dest, err := lib.ParseDest(url)
	if err != nil {
		g.log.Error("invalid blockchain contract destination", err)
		return contractData, err
	}

	state, price, limit, speed, length, startingBlockTimestamp, buyer, seller, _, err := instance.GetPublicVariables(&bind.CallOpts{})
	if err != nil {
		g.log.Error(err)
		return contractData, err
	}

	contractData = NewContractData(contractAddress, buyer, seller, state, price.Int64(), limit.Int64(), speed.Int64(), length.Int64(), startingBlockTimestamp.Int64(), dest)

	// TODO: uncomment when encryption is enabled on frontend
	// return g.decryptDest(url)

	return contractData, nil
}

func (g *EthereumGateway) ReadContracts(walletAddr interop.BlockchainAddress, isBuyer bool) ([]interop.BlockchainAddress, error) {
	hashrateContractAddresses, err := g.cloneFactory.GetContractList(&bind.CallOpts{})
	if err != nil {
		g.log.Error(err)
		return nil, err
	}

	var walletContractAddresses []common.Address

	// parse existing hashrate contracts for ones that belong to seller or buyer
	for i := range hashrateContractAddresses {
		hashrateContractInstance, err := implementation.NewImplementation(hashrateContractAddresses[i], g.client)
		if err != nil {
			g.log.Error(err)
			return nil, err
		}
		if isBuyer {
			hashrateContractBuyer, err := hashrateContractInstance.Buyer(nil)
			if err != nil {
				g.log.Error(err)
				return nil, err
			}
			if hashrateContractBuyer == walletAddr {
				walletContractAddresses = append(walletContractAddresses, hashrateContractAddresses[i])
			}
		} else {
			hashrateContractSeller, err := hashrateContractInstance.Seller(nil)
			if err != nil {
				g.log.Error(err)
				return nil, err
			}
			if hashrateContractSeller == walletAddr {
				walletContractAddresses = append(walletContractAddresses, hashrateContractAddresses[i])
			}
		}
	}

	return walletContractAddresses, nil
}

// SetContractCloseOut closes the contract with specified closeoutType
func (g *EthereumGateway) SetContractCloseOut(fromAddress string, contractAddress string, closeoutType int64) error {
	g.startCloseout <- &closeout{fromAddress, contractAddress, closeoutType}

	err := <-g.endCloseout

	return err
}

func (g *EthereumGateway) setContractCloseOut(fromAddress string, contractAddress string, closeoutType int64) error {
	g.log.Debugf("starting closeout, %v; %v; %v", fromAddress, contractAddress, closeoutType)
	ctx := context.TODO()

	instance, err := implementation.NewImplementation(common.HexToAddress(contractAddress), g.client)
	if err != nil {
		g.log.Error(err)
		return err
	}

	privateKey, err := crypto.HexToECDSA(g.sellerPrivateKeyString)
	if err != nil {
		g.log.Error(err)
		return err
	}

	chainId, err := g.client.ChainID(ctx)
	if err != nil {
		g.log.Error(err)
		return err
	}
	//TODO: deal with likely gasPrice issue so our transaction processes before another pending nonce.
	// gasPrice, err := g.client.SuggestGasPrice(ctx)
	// if err != nil {
	// 	g.log.Error(err)
	// 	return err
	// }

	nonce, err := g.client.PendingNonceAt(ctx, common.HexToAddress(fromAddress))

	if err != nil {
		return err
	}

	options, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return err
	}

	options.GasLimit = uint64(3000000) // in units
	options.Value = big.NewInt(0)      // in wei
	// options.GasPrice = gasPrice
	options.Nonce = big.NewInt(int64(nonce))
	g.log.Debugf("closeout type: %v; nonce: %v", closeoutType, nonce)

	//TODO: retry if price is too low
	tx, err := instance.SetContractCloseOut(options, big.NewInt(closeoutType))

	if err != nil {
		g.log.Errorf("cannot close transaction: %s tx: %s fromAddr: %s contractAddr: %s", err, tx, fromAddress, contractAddress)
		return err
	}
	time.Sleep(30 * time.Second)
	g.log.Infof("contract %s closed, tx: %s", contractAddress, tx.Hash().Hex())

	g.log.Debugf("ending closeout, %v; %v; %v", fromAddress, contractAddress, closeoutType)
	return nil
}

func (g *EthereumGateway) GetBalanceWei(ctx context.Context, addr common.Address) (*big.Int, error) {
	return g.client.BalanceAt(ctx, addr, nil)
}

// decryptDest decrypts destination uri which is encrypted with private key of the contract creator
func (g *EthereumGateway) decryptDest(encryptedDestUrl string) (string, error) {
	privateKey, err := crypto.HexToECDSA(g.sellerPrivateKeyString)
	if err != nil {
		g.log.Error(err)
		return "", err
	}

	privateKeyECIES := ecies.ImportECDSA(privateKey)
	destUrlBytes, err := hex.DecodeString(encryptedDestUrl)
	if err != nil {
		g.log.Error(err)
		return "", err
	}

	decryptedDestUrlBytes, err := privateKeyECIES.Decrypt(destUrlBytes, nil, nil)
	if err != nil {
		g.log.Error(err)
		return "", err
	}

	return string(decryptedDestUrlBytes), nil
}
