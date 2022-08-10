//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/wire"
	"gitlab.com/TitanInd/hashrouter/api"
	"gitlab.com/TitanInd/hashrouter/app"
	"gitlab.com/TitanInd/hashrouter/config"
	"gitlab.com/TitanInd/hashrouter/contractmanager"
	"gitlab.com/TitanInd/hashrouter/contractmanager/blockchain"
	"gitlab.com/TitanInd/hashrouter/eventbus"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/lib"
	"gitlab.com/TitanInd/hashrouter/miner"
	"gitlab.com/TitanInd/hashrouter/tcpserver"
	"os"
)

const VERSION = "0.01"

func main() {
	appInstance, err := InitApp()
	if err != nil {
		panic(err)
	}

	appInstance.Run()
}

var networkSet = wire.NewSet(provideTCPServer, provideServer)
var protocolSet = wire.NewSet(miner.NewMinerRepo, provideMinerController, eventbus.NewEventBus, provideConnectionsService)
var contractsSet = wire.NewSet(blockchain.NewBlockchainWallet, provideEthClient, blockchain.NewBlockchainGateway, provideContractFactory, contractmanager.NewContractsGateway, contractmanager.NewNodeOperator, contractmanager.NewContractsService, provideSellerContractManager)
var hashrateCalculationSet = wire.NewSet(provideHashrateCalculator)

func InitApp() (*app.App, error) {
	wire.Build(
		provideConfig,
		provideLogger,
		networkSet,
		protocolSet,
		hashrateCalculationSet,
		contractsSet,
		wire.Struct(new(app.App), "*"),
	)
	return nil, nil
}

func provideContractFactory() interfaces.IContractFactory {
	return nil
}

func provideConnectionsService() interfaces.IConnectionsService {
	return nil
}

func provideHashrateCalculator() interfaces.IValidatorsService {
	return nil
}

func provideMinerController(cfg *config.Config, l interfaces.ILogger, repo *miner.MinerRepo) *miner.MinerController {
	return miner.NewMinerController(cfg.Pool.Address, cfg.Pool.User, cfg.Pool.Password, repo, l)
}

func provideTCPServer(cfg *config.Config, l interfaces.ILogger) *tcpserver.TCPServer {
	return tcpserver.NewTCPServer(cfg.Proxy.Address, l)
}

func provideServer(cfg *config.Config, l interfaces.ILogger, ph *miner.MinerController) *api.Server {
	return api.NewServer(cfg.Web.Address, l, ph)
}

func provideEthClient(cfg *config.Config) (*ethclient.Client, error) {
	return contractmanager.NewEthClient(cfg.EthNode.Address)
}

func provideSellerContractManager(
	contractsService interfaces.IContractsService,
	cfg *config.Config,
	em interfaces.IEventManager,
	factory interfaces.IContractFactory,
	ethClient *ethclient.Client,
	logger interfaces.ILogger,
	nodeOperator *contractmanager.NodeOperator,
	wallet interfaces.IBlockchainWallet,
) (interfaces.ContractManager, error) {
	return contractmanager.NewContractManager(context.TODO(), contractsService, logger, cfg, em, factory, ethClient, nodeOperator, wallet) //cfg.Contract.Address,  cfg.Contract.IsBuyer
}

func provideLogger(cfg *config.Config) (interfaces.ILogger, error) {
	return lib.NewLogger(cfg.Log.Syslog)
}

func provideConfig() (*config.Config, error) {
	var cfg config.Config
	return &cfg, config.LoadConfig(&cfg, &os.Args)
}
