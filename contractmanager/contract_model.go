package contractmanager

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/TitanInd/hashrouter/blockchain"
	"gitlab.com/TitanInd/hashrouter/constants"
	"gitlab.com/TitanInd/hashrouter/hashrate"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/lib"
	"golang.org/x/sync/errgroup"
)

// TODO: consider renaming to ContractInternalState to avoid collision with the state which is in blockchain
type ContractState = uint8

const (
	ContractStateAvailable ContractState = iota // contract was created and the system is following its updates
	ContractStatePurchased                      // contract was purchased but not yet picked up by miners
	ContractStateRunning                        // contract is fulfilling
	ContractStateClosed                         // contract is closed
)

// BTCHashrateContract represents the collection of mining resources (collection of miners / parts of the miners) that work to fulfill single contract and monotoring tools of their performance
type BTCHashrateContract struct {
	// dependencies
	blockchain      interfaces.IBlockchainGateway
	globalScheduler *GlobalSchedulerService

	data                  blockchain.ContractData
	FullfillmentStartTime *time.Time
	isBuyer               bool

	state ContractState // internal state of the contract (within hashrouter)

	hashrate *hashrate.Hashrate // the counter of single contract
	minerIDs []string           // miners involved in fulfilling this contract

	log interfaces.ILogger
}

func NewContract(data blockchain.ContractData, blockchain interfaces.IBlockchainGateway, globalScheduler *GlobalSchedulerService, log interfaces.ILogger, hr *hashrate.Hashrate, isBuyer bool) *BTCHashrateContract {
	if hr == nil {
		hr = hashrate.NewHashrate(log)
	}
	return &BTCHashrateContract{
		blockchain:      blockchain,
		data:            data,
		hashrate:        hr,
		log:             log,
		isBuyer:         isBuyer,
		globalScheduler: globalScheduler,
		state:           ContractStateAvailable,
	}
}

// Runs goroutine that monitors the contract events and replace the miners which are out
func (c *BTCHashrateContract) Run(ctx context.Context) error {
	g, subCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return c.listenContractEvents(subCtx)
	})

	return g.Wait()
}

// Ignore checks if contract should be ignored by the node
func (c *BTCHashrateContract) Ignore(walletAddress common.Address, defaultDest lib.Dest) bool {
	if c.isBuyer {
		if c.data.State == blockchain.ContractBlockchainStateAvailable || c.data.Buyer != walletAddress {
			return true
		}
		// buyer node points contracts to default
		c.setDestToDefault(defaultDest)
		return false
	}

	if c.data.State == blockchain.ContractBlockchainStateRunning || c.data.Seller != walletAddress {
		return true
	}
	return false
}

// Sets contract dest to default dest for buyer node
func (c *BTCHashrateContract) setDestToDefault(defaultDest lib.Dest) {
	c.data.Dest = defaultDest
}

func (c *BTCHashrateContract) listenContractEvents(ctx context.Context) error {
	eventsCh, sub, err := c.blockchain.SubscribeToContractEvents(ctx, common.HexToAddress(c.GetAddress()))

	if err != nil {
		return fmt.Errorf("cannot subscribe for contract events %w", err)
	}

	go func() {
		err = c.fulfillBuyerContract(ctx)
		if err != nil {
			err := c.Close()
			c.log.Error(err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.log.Errorf("Unsubscribing from contract %v", c.GetID())
			sub.Unsubscribe()
			return ctx.Err()
		case err := <-sub.Err():

			c.log.Errorf("Contract subscription error %v", c.GetID())
			return err
		case e := <-eventsCh:
			eventHex := e.Topics[0].Hex()

			// _ /* payloadHex*/ := e.Topics[1].Hex()

			switch eventHex {
			case blockchain.ContractPurchasedHex:
				// buyerAddr := common.HexToAddress(payloadHex)
				// get updated contract information fields: buyer and dest
				err := c.LoadBlockchainContract()

				if err != nil {
					continue
				}

				// use the same group to fail together with main goroutine
				// seller node responds to purchase events off the contract
				go func() {
					err = c.FulfillContract(ctx)
					if err != nil {
						c.log.Error(err)
						err := c.Close()
						if err != nil {
							c.log.Error(err)
						}
					}
				}()
				continue

			case blockchain.ContractCipherTextUpdatedHex:
			case blockchain.ContractPurchaseInfoUpdatedHex:
				err := c.LoadBlockchainContract()

				if err != nil {
					continue
				}

			case blockchain.ContractClosedSigHex:
				c.log.Info("received contract closed event", c.data.Addr)
				c.Stop()
				continue
			}

		}
	}
}
func (c *BTCHashrateContract) LoadBlockchainContract() error {
	data, err := c.blockchain.ReadContract(c.data.Addr)
	if err != nil {
		c.log.Error("cannot read contract", err)
		return err
	}
	// TODO guard it
	contractData, ok := data.(blockchain.ContractData)

	if !ok {
		return fmt.Errorf("failed to load blockhain data: %#+v", c.data.Addr)
	}

	c.data = contractData

	return nil
}

func (c *BTCHashrateContract) fulfillBuyerContract(ctx context.Context) error {
	if c.data.State != blockchain.ContractBlockchainStateRunning {
		return nil // not buyer in buyer case
	}
	time.Sleep(constants.ValidationBufferPeriod)
	return c.FulfillContract(ctx)
}

func (c *BTCHashrateContract) FulfillContract(ctx context.Context) error {
	c.state = ContractStatePurchased

	if c.ContractIsExpired() {
		c.log.Warn("contract is expired %s", c.GetID())
		return fmt.Errorf("contract is expired")
	}
	//race condition here for some contract closeout scenarios
	// initialization cycle waits for hashpower to be available
	// for {
	err := c.StartHashrateAllocation()
	if err != nil {
		return err
	}

	// break
	// select {
	// case <-ctx.Done():
	// 	c.log.Errorf("contract context canceled while waiting for hashpower: %s", ctx.Err().Error())
	// 	return ctx.Err()
	// case <-time.After(30 * time.Second):
	// }
	// }

	// running cycle checks combination every N seconds
	for {
		c.log.Debugf("Checking if contract is ready for allocation: %v", c.GetID())

		if c.ContractIsExpired() {
			c.log.Info("contract time ended, or state is closed, closing...", c.GetID())
			return fmt.Errorf("contract is expired")
		}

		// TODO hashrate monitoring
		c.log.Infof("contract (%s) is running for %.0f seconds", c.GetID(), time.Since(*c.GetStartTime()).Seconds())

		minerIDs, err := c.globalScheduler.UpdateCombination(ctx, c.minerIDs, c.GetHashrateGHS(), c.GetDest(), c.GetID())
		if err != nil {
			return fmt.Errorf("contract is expired")
		} else {
			c.minerIDs = minerIDs
		}

		select {
		case <-ctx.Done():
			c.log.Errorf("contract context done while waiting for running contract to finish: %v", ctx.Err().Error())
			return ctx.Err()
		case <-time.After(30 * time.Second):
		}
	}
}

func (c *BTCHashrateContract) ContractIsReady() bool {
	return !c.ContractIsExpired()
}

func (c *BTCHashrateContract) StartHashrateAllocation() error {
	c.state = ContractStateRunning

	minerList, err := c.globalScheduler.Allocate(c.GetID(), c.GetHashrateGHS(), c.data.Dest)

	if err != nil {
		return err
	}

	c.minerIDs = minerList.IDs()

	c.log.Infof("fulfilling contract %s; expires at %v", c.GetID(), c.GetEndTime())

	return nil
}

func (c *BTCHashrateContract) ContractIsExpired() bool {
	endTime := c.GetEndTime()
	if endTime == nil {
		return false
	}
	return time.Now().After(*endTime)
}

func (c *BTCHashrateContract) Close() error {
	c.log.Debugf("closing contract %v", c.GetID())
	c.Stop()

	closeoutAccount := c.GetCloseoutAccount()

	err := c.blockchain.SetContractCloseOut(closeoutAccount, c.GetAddress(), int64(c.GetCloseoutType()))
	c.log.Debugf("exited closeout")
	if err != nil {
		c.log.Error("cannot close contract", err)
		return err
	}

	return nil
}

// Stops fulfilling the contract by miners
func (c *BTCHashrateContract) Stop() {

	c.log.Infof("Attempting to stop contract %v; with state %v", c.GetID(), c.state)
	if c.state == ContractStateRunning || c.state == ContractStatePurchased {

		c.log.Infof("Stopping contract %v", c.GetID())
		c.state = ContractStateAvailable
		c.globalScheduler.DeallocateContract(c.minerIDs, c.GetID())
	} else {
		c.log.Warnf("contract (%s) is not running", c.GetID())
	}
}

func (c *BTCHashrateContract) GetBuyerAddress() string {
	return c.data.Buyer.String()
}

func (c *BTCHashrateContract) GetSellerAddress() string {
	return c.data.Seller.String()
}

func (c *BTCHashrateContract) GetID() string {
	return c.GetAddress()
}

func (c *BTCHashrateContract) GetAddress() string {
	return c.data.Addr.String()
}

func (c *BTCHashrateContract) GetHashrateGHS() int {
	return int(c.data.Speed / int64(math.Pow10(9)))
}

func (c *BTCHashrateContract) GetDuration() time.Duration {
	return time.Duration(c.data.Length) * time.Second
}

func (c *BTCHashrateContract) GetStartTime() *time.Time {
	startTime := time.Unix(c.data.StartingBlockTimestamp, 0)

	return &startTime
}

func (c *BTCHashrateContract) GetEndTime() *time.Time {
	endTime := c.GetStartTime().Add(c.GetDuration())
	return &endTime
}

func (c *BTCHashrateContract) GetState() ContractState {
	return c.state
}

func (c *BTCHashrateContract) GetDest() lib.Dest {
	return c.data.Dest
}

func (c *BTCHashrateContract) GetCloseoutType() constants.CloseoutType {
	if c.isBuyer {
		return constants.CloseoutTypeCancel
	}
	return constants.CloseoutTypeWithoutClaim
}

func (c *BTCHashrateContract) GetCloseoutAccount() string {
	if c.isBuyer {
		return c.GetBuyerAddress()
	}
	return c.GetSellerAddress()
}

var _ interfaces.IModel = (*BTCHashrateContract)(nil)
