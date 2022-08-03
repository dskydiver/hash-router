package protocol

import (
	"context"

	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/protocol/stratumv1_message"
	"gitlab.com/TitanInd/hashrouter/validatorv2"
)

type stratumV1MinerModel struct {
	pool      StratumV1DestConn
	miner     StratumV1SourceConn
	validator *validatorv2.Validator
	log       interfaces.ILogger
}

func NewStratumV1MinerModel(poolPool StratumV1DestConn, miner StratumV1SourceConn, validator *validatorv2.Validator, log interfaces.ILogger) *stratumV1MinerModel {
	return &stratumV1MinerModel{
		pool:      poolPool,
		miner:     miner,
		validator: validator,
		log:       log,
	}
}

func (s *stratumV1MinerModel) Run() error {
	s.log.Info("proxying started")
	errCh := make(chan error)
	go func() {
		for {
			msg, err := s.pool.Read(context.TODO())
			if err != nil {
				s.log.Error(err)
				errCh <- err
				return
			}

			s.poolInterceptor(msg)

			err = s.miner.Write(context.TODO(), msg)
			if err != nil {
				s.log.Error(err)
				errCh <- err
				return
			}
		}
	}()

	go func() {
		for {
			msg, err := s.miner.Read(context.TODO())
			if err != nil {
				s.log.Error(err)
				errCh <- err
				return
			}

			s.minerInterceptor(msg)

			err = s.pool.Write(context.TODO(), msg)
			if err != nil {
				s.log.Error(err)
				errCh <- err
				return
			}
		}
	}()

	go func() {
		err := s.validator.Run(context.TODO())
		if err != nil {
			s.log.Error(err)
			errCh <- err
			return
		}
	}()

	return <-errCh
}

func (s *stratumV1MinerModel) minerInterceptor(msg stratumv1_message.MiningMessageGeneric) {
	switch m := msg.(type) {
	case *stratumv1_message.MiningSubmit:
		s.validator.IncomingHash(m.GetWorkerName(), m.GetNonce(), m.GetNtime())
		s.validator.UpdateHashrate()
	}
}

func (s *stratumV1MinerModel) poolInterceptor(msg stratumv1_message.MiningMessageGeneric) {
	switch m := msg.(type) {
	case *stratumv1_message.MiningNotify:
		s.validator.OnMinerNotify(m.GetVersion(), m.GetPrevBlockHash(), m.GetNbits(), m.GetNtime(), m.GetMerkel())
	case *stratumv1_message.MiningSetDifficulty:
		//TODO: some pools return difficulty in float, decide if we need that kind of precision
		s.validator.SetNewDiff(int(m.GetDifficulty()))
	}
}

func (s *stratumV1MinerModel) ChangeDest(addr string, authUser string, authPwd string) error {
	err := s.pool.SetDest(addr, authUser, authPwd)
	return err
}

func (s *stratumV1MinerModel) GetID() string {
	return s.miner.GetID()
}

func (s *stratumV1MinerModel) GetHashRate() int {
	return s.validator.GetHashrate()
}
