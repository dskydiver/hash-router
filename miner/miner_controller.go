package miner

import (
	"context"
	"net"
	"strings"
	"time"

	"gitlab.com/TitanInd/hashrouter/hashrate"
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/protocol"
	"gitlab.com/TitanInd/hashrouter/protocol/stratumv1_message"
	"gitlab.com/TitanInd/hashrouter/tcpserver"
)

type MinerController struct {
	defaultDest        interfaces.IDestination
	collection         interfaces.ICollection[MinerScheduler]
	log                interfaces.ILogger
	logStratum         bool
	minerVettingPeriod time.Duration
	poolMinDuration    time.Duration
	poolMaxDuration    time.Duration
}

func NewMinerController(defaultDest interfaces.IDestination, collection interfaces.ICollection[MinerScheduler], log interfaces.ILogger, logStratum bool, minerVettingPeriod time.Duration, poolMinDuration, poolMaxDuration time.Duration) *MinerController {
	return &MinerController{
		defaultDest:        defaultDest,
		log:                log,
		collection:         collection,
		logStratum:         logStratum,
		minerVettingPeriod: minerVettingPeriod,
		poolMinDuration:    poolMinDuration,
		poolMaxDuration:    poolMaxDuration,
	}
}

func (p *MinerController) HandleConnection(ctx context.Context, incomingConn net.Conn) error {
	p.log.Infof("incoming miner connection: %s", incomingConn.RemoteAddr().String())
	// TODO: peek if incoming connection is stratum connection

	buffered := tcpserver.NewBufferedConn(incomingConn)
	bytes, err := tcpserver.PeekJSON(buffered)
	if err != nil {
		err2 := buffered.Close()
		if err2 != nil {
			return err2
		}
		return err
	}
	peakedMsg := strings.ToLower(string(bytes))

	if !(strings.Contains(peakedMsg, "id") &&
		strings.Contains(peakedMsg, "mining") &&
		strings.Contains(peakedMsg, "params")) {
		p.log.Infof("invalid incoming message: %s", peakedMsg)
		err := buffered.Close()
		return err
	}

	incomingConn = buffered

	logMiner := p.log.Named(incomingConn.RemoteAddr().String())

	poolPool := protocol.NewStratumV1PoolPool(logMiner, p.logStratum)
	err = poolPool.SetDest(p.defaultDest, nil)
	if err != nil {
		p.log.Error(err)
		return err
	}
	extranonce, size := poolPool.GetExtranonce()
	msg := stratumv1_message.NewMiningSubscribeResult(extranonce, size)
	miner := protocol.NewStratumV1MinerConn(incomingConn, logMiner, msg, p.logStratum, time.Now())
	validator := hashrate.NewHashrate(logMiner)
	minerModel := protocol.NewStratumV1MinerModel(poolPool, miner, validator, logMiner)

	destSplit := NewDestSplit()

	minerScheduler := NewOnDemandMinerScheduler(minerModel, destSplit, logMiner, p.defaultDest, p.minerVettingPeriod, p.poolMinDuration, p.poolMaxDuration)
	// try to connect to dest before running

	p.collection.Store(minerScheduler)
	defer p.collection.Delete(minerScheduler.GetID())

	return minerScheduler.Run(ctx)
}

func (p *MinerController) ChangeDestAll(dest interfaces.IDestination) error {
	p.collection.Range(func(miner MinerScheduler) bool {
		p.log.Infof("changing pool to %s for minerID %s", dest.GetHost(), miner.GetID())

		_, err := miner.Allocate("API_TEST", 1, dest)

		return err == nil
	})

	return nil
}
