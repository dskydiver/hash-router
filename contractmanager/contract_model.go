package contractmanager

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/hashrouter/blockchain"
	"gitlab.com/TitanInd/hashrouter/hashrate"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/lib"
	"golang.org/x/sync/errgroup"
)

// ContractState defines the state of the subset of contracts that system is interested in.
// It does not maps directly to blockchain ContractState
// TODO: consider renaming to ContractInternalState to avoid collision with the state which is in blockchain
type ContractState = uint8

const (
	ContractStateCreated   ContractState = iota // contract was created and the system is following its updates
	ContractStatePurchased                      // contract was purchased but not yet picked up by miners
	ContractStateRunning                        // contract is fulfilling
	ContractStateClosed                         // contract is closed
)

// Contract represents the collection of mining resources (collection of miners / parts of the miners) that work to fulfill single contract and monotoring tools of their performance
type Contract struct {
	// dependencies
	blockchain      *blockchain.EthereumGateway
	globalScheduler *GlobalSchedulerService

	data                  blockchain.ContractData
	FullfillmentStartTime int64
	closeoutType          blockchain.CloseoutType

	state            ContractState
	contractClosedCh chan struct{}

	eventsCh chan blockchain.BlockchainEvent
	eventSub blockchain.BlockchainEventSubscription

	hashrate    *hashrate.Hashrate // the counter of single contract
	combination HashrateList       // combination of full/partially allocated miners fulfilling the contract

	log interfaces.ILogger
}

func NewContract(data blockchain.ContractData, blockchain *blockchain.EthereumGateway, globalScheduler *GlobalSchedulerService, log interfaces.ILogger, hr *hashrate.Hashrate) *Contract {
	if hr == nil {
		hr = hashrate.NewHashrate(log, hashrate.EMA_INTERVAL)
	}
	return &Contract{
		blockchain:            blockchain,
		data:                  data,
		hashrate:              hr,
		log:                   log,
		contractClosedCh:      make(chan struct{}),
		closeoutType:          2,
		globalScheduler:       globalScheduler,
		FullfillmentStartTime: 0,
	}
}

// Runs goroutine that monitors the contract events and replace the miners which are out
func (c *Contract) Run(ctx context.Context) error {
	g, subCtx := errgroup.WithContext(ctx)

	// if proxy started after the contract was purchased and wasn't able to pick up event
	c.log.Infof("contract is being listened %s %v", c.GetID(), c.data.State)
	if c.data.State == blockchain.ContractBlockchainStateRunning {
		g.Go(func() error {
			return c.fulfillContract(subCtx)
		})
	}

	g.Go(func() error {
		return c.listenContractEvents(subCtx, g)
	})

	g.Go(func() error {
		for {
			// <-c.contractClosedCh
			// c.log.Infof("contract closed")
		}
	})

	return g.Wait()
}

func (c *Contract) listenContractEvents(ctx context.Context, errGroup *errgroup.Group) error {
	eventsCh, sub, err := c.blockchain.SubscribeToContractEvents(ctx, common.HexToAddress(c.GetAddress()))
	if err != nil {
		return fmt.Errorf("cannot subscribe for contract events %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return ctx.Err()
		case err := <-sub.Err():
			return err
		case e := <-eventsCh:
			eventHex := e.Topics[0].Hex()

			// _ /* payloadHex*/ := e.Topics[1].Hex()

			switch eventHex {
			case blockchain.ContractPurchasedHex:
				c.state = ContractStatePurchased
				// buyerAddr := common.HexToAddress(payloadHex)
				// get updated contract information fields: buyer and dest
				data, err := c.blockchain.ReadContract(c.data.Addr)
				if err != nil {
					c.log.Error("cannot read contract", err)
					continue
				}
				// TODO guard it
				c.data = data

				// use the same group to fail together with main goroutine
				errGroup.Go(func() error {
					return c.fulfillContract(ctx)
				})

			case blockchain.ContractCipherTextUpdatedHex:
			case blockchain.ContractPurchaseInfoUpdatedHex:
				data, err := c.blockchain.ReadContract(c.data.Addr)
				if err != nil {
					c.log.Error("cannot read contract", err)
					continue
				}
				// TODO guard it
				c.data = data

			case blockchain.ContractClosedSigHex:
				c.log.Info("received contract closed event", c.data.Addr)
				c.Stop()
				return nil
			}

		}
	}
}

func (c *Contract) fulfillContract(ctx context.Context) error {
	c.state = ContractStateRunning

allocationBlock:
	for {
		if !c.ContractIsExpired() {

			err := c.StartHashrateAllocation()

			if err != nil {
				c.log.Warn("cannot allocate hashrate", err)
				select {
				case <-ctx.Done():
					c.log.Errorf("contract context done while waiting for hashpower: %v", ctx.Err().Error())
					return ctx.Err()
				case <-time.After(30 * time.Second):
				}
				continue
			}
		} else if c.ContractIsExpired() {
			c.log.Info("contract time ended, closing...", c.GetID())

			//TODO: make sure this is updated so that we continue listening for contract events.
			err := c.blockchain.SetContractCloseOut(c.data.Seller.Hex(), c.GetAddress(), c.closeoutType)
			if err != nil {
				c.log.Error("cannot close contract", err)
			}
			return nil
		}
		// TODO hashrate monitoring
		c.log.Info("contract running...", c.GetID())

		select {
		case <-ctx.Done():
			c.log.Errorf("contract context done while waiting for running contract to finish: %v", ctx.Err().Error())
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func (c *Contract) StartHashrateAllocation() error {

	minerList, err := c.globalScheduler.Allocate(c.GetHashrateGHS(), c.data.Dest)

	if err != nil {
		return err
	}

	c.combination = minerList
	c.FullfillmentStartTime = time.Now().Unix()

	c.log.Info("fulfilling contract %s; expires at %v", c.GetID(), c.GetEndTime())

	return nil
}

func (c *Contract) ContractIsExpired() bool {
	return c.FullfillmentStartTime != 0 && time.Now().Unix() > c.GetEndTime().Unix()
}

// Stops fulfilling the contract by miners
func (c *Contract) Stop() {
	for _, miner := range c.combination {
		ok := miner.SplitPtr.Deallocate()
		if !ok {
			c.log.Error("miner split not found during STOP . minerID: %s, contractID: %s", miner.GetSourceID(), c.GetID())
		}
	}

	c.FullfillmentStartTime = 0
	// close(c.contractClosedCh)
}

func (c *Contract) GetBuyerAddress() string {
	return c.data.Buyer.String()
}

func (c *Contract) GetSellerAddress() string {
	return c.data.Seller.String()
}

func (c *Contract) GetID() string {
	return c.GetAddress()
}

func (c *Contract) GetAddress() string {
	return c.data.Addr.String()
}

func (c *Contract) GetHashrateGHS() int {
	return int(c.data.Speed / int64(math.Pow10(9)))
}

func (c *Contract) GetStartTime() time.Time {
	return time.Unix(c.data.StartingBlockTimestamp, 0)
}

func (c *Contract) GetEndTime() time.Time {
	return time.Unix(c.FullfillmentStartTime+c.data.Length, 0)
}

func (c *Contract) GetState() ContractState {
	return c.state
}

func (c *Contract) GetDest() lib.Dest {
	return c.data.Dest
}

var _ interfaces.IModel = (*Contract)(nil)
