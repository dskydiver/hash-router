package miner

import (
	"context"
	"net"

	"go.uber.org/zap"

	"gitlab.com/TitanInd/hashrouter/protocol"
	"gitlab.com/TitanInd/hashrouter/protocol/stratumv1_message"
)

type MinerController struct {
	poolAddr     string
	poolUser     string
	poolPassword string

	repo *MinerRepo

	log *zap.SugaredLogger
}

func NewMinerController(poolAddr string, poolUser string, poolPassword string, repo *MinerRepo, log *zap.SugaredLogger) *MinerController {
	return &MinerController{
		poolAddr:     poolAddr,
		poolUser:     poolUser,
		poolPassword: poolPassword,
		log:          log,
		repo:         repo,
	}
}

func (p *MinerController) ConnectionHandler(ctx context.Context, incomingConn net.Conn) error {
	poolPool := protocol.NewStratumV1PoolPool(p.log)
	err := poolPool.SetDest(p.poolAddr, p.poolUser, p.poolPassword)
	if err != nil {
		p.log.Error(err)
		return err
	}
	extranonce, size := poolPool.GetExtranonce()
	msg := stratumv1_message.NewMiningSubscribeResult(extranonce, size)
	miner := protocol.NewStratumV1Miner(incomingConn, p.log, msg)
	manager := protocol.NewStratumV1MinerModel(poolPool, miner, p.log)
	// try to connect to dest before running

	p.repo.Store(manager)

	return manager.Run()

	// return nil
}

func (p *MinerController) ChangeDestAll(addr string, username string, pwd string) error {
	p.repo.Range(func(miner MinerModel) bool {
		p.log.Infof("changing pool to %s for minerID %s", addr, miner.GetID())

		err := miner.ChangeDest(addr, username, pwd)
		if err != nil {
			p.log.Errorf("error changing pool %w", err)
		} else {
			p.log.Info("Pool changed for minerid %s", miner.GetID())
		}
		return true
	})

	return nil
}
