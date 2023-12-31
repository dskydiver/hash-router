//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"gitlab.com/TitanInd/hashrouter/api"
	"gitlab.com/TitanInd/hashrouter/app"
	"gitlab.com/TitanInd/hashrouter/blockchain"
	"gitlab.com/TitanInd/hashrouter/config"
	"gitlab.com/TitanInd/hashrouter/contractmanager"
	"gitlab.com/TitanInd/hashrouter/eventbus"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/lib"
	"gitlab.com/TitanInd/hashrouter/miner"
	"gitlab.com/TitanInd/hashrouter/tcpserver"
)

const VERSION = "0.01"

func main() {
	appInstance, err := InitApp()
	if err != nil {
		panic(err)
	}

	appInstance.Run()
}

var networkSet = wire.NewSet(provideTCPServer, provideApiServer)
var protocolSet = wire.NewSet(provideMinerCollection, provideMinerController, eventbus.NewEventBus)
var contractsSet = wire.NewSet(provideGlobalScheduler, provideContractCollection, provideEthClient, provideEthWallet, provideEthGateway, provideContractManager)

// TODO: make sure all providers initialized
func InitApp() (*app.App, error) {
	wire.Build(
		provideConfig,
		provideLogger,
		provideApiController,
		networkSet,
		protocolSet,
		contractsSet,
		wire.Struct(new(app.App), "*"),
	)
	return nil, nil
}

func provideGlobalScheduler(cfg *config.Config, miners interfaces.ICollection[miner.MinerScheduler], log interfaces.ILogger) *contractmanager.GlobalSchedulerService {
	return contractmanager.NewGlobalScheduler(miners, log, cfg.Pool.MinDuration, cfg.Pool.MaxDuration)
}

func provideMinerCollection() interfaces.ICollection[miner.MinerScheduler] {
	return miner.NewMinerCollection()
}

func provideContractCollection() interfaces.ICollection[contractmanager.IContractModel] {
	return contractmanager.NewContractCollection()
}

func provideMinerController(cfg *config.Config, l interfaces.ILogger, repo interfaces.ICollection[miner.MinerScheduler]) (*miner.MinerController, error) {
	destination, err := lib.ParseDest(cfg.Pool.Address)
	if err != nil {
		return nil, err
	}

	return miner.NewMinerController(destination, repo, l, cfg.Proxy.LogStratum, cfg.Miner.VettingDuration, cfg.Pool.MinDuration, cfg.Pool.MaxDuration), nil
}

func provideApiController(miners interfaces.ICollection[miner.MinerScheduler], contracts interfaces.ICollection[contractmanager.IContractModel], log interfaces.ILogger, gs *contractmanager.GlobalSchedulerService) *gin.Engine {
	return api.NewApiController(miners, contracts, log, gs)
}

func provideTCPServer(cfg *config.Config, l interfaces.ILogger) *tcpserver.TCPServer {
	return tcpserver.NewTCPServer(cfg.Proxy.Address, l)
}

func provideApiServer(cfg *config.Config, l interfaces.ILogger, controller *gin.Engine) *api.Server {
	return api.NewServer(cfg.Web.Address, l, controller)
}

func provideEthClient(cfg *config.Config, log interfaces.ILogger) (*ethclient.Client, error) {
	return blockchain.NewEthClient(cfg.EthNode.Address, log)
}

func provideEthWallet(cfg *config.Config) (*blockchain.EthereumWallet, error) {
	return blockchain.NewEthereumWallet(cfg.Contract.Mnemonic, cfg.Contract.AccountIndex, cfg.Contract.WalletPrivateKey, cfg.Contract.WalletAddress)
}

func provideEthGateway(cfg *config.Config, ethClient *ethclient.Client, ethWallet *blockchain.EthereumWallet, log interfaces.ILogger) (*blockchain.EthereumGateway, error) {
	backoff := lib.NewLinearBackoff(2*time.Second, nil, lib.Of(15*time.Second))
	g, err := blockchain.NewEthereumGateway(ethClient, ethWallet.GetPrivateKey(), cfg.Contract.Address, log, backoff)
	if err != nil {
		return nil, err
	}

	balanceWei, err := g.GetBalanceWei(context.Background(), ethWallet.GetAccountAddress())
	if err != nil {
		return nil, err
	}
	log.Infof("account %s balance %.4f ETH", ethWallet.GetAccountAddress(), lib.WeiToEth(balanceWei))

	return g, nil
}

func provideContractManager(
	cfg *config.Config,
	ethGateway *blockchain.EthereumGateway,
	ethWallet *blockchain.EthereumWallet,
	globalScheduler *contractmanager.GlobalSchedulerService,
	contracts interfaces.ICollection[contractmanager.IContractModel],
	log interfaces.ILogger,
) (*contractmanager.ContractManager, error) {
	destination, err := lib.ParseDest(cfg.Pool.Address)
	if err != nil {
		return nil, err
	}

	return contractmanager.NewContractManager(ethGateway, globalScheduler, log, contracts, ethWallet.GetAccountAddress(), ethWallet.GetPrivateKey(), cfg.Contract.IsBuyer, destination), nil
}

func provideLogger(cfg *config.Config) (interfaces.ILogger, error) {
	return lib.NewLogger(cfg.Environment == "production")
}

func provideConfig() (*config.Config, error) {
	var cfg config.Config
	return &cfg, config.LoadConfig(&cfg, &os.Args)
}
