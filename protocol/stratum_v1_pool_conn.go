package protocol

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"gitlab.com/TitanInd/hashrouter/interfaces"
	"gitlab.com/TitanInd/hashrouter/lib"
	"gitlab.com/TitanInd/hashrouter/protocol/stratumv1_message"
	"go.uber.org/atomic"
)

// StratumV1PoolConn represents connection to the pool on the protocol level
type StratumV1PoolConn struct {
	dest interfaces.IDestination // destination TODO: replace it with value type, instead of pointer

	conn net.Conn // tcp connection

	notifyMsgs    []*stratumv1_message.MiningNotify      // recent relevant notify messages, (respects stratum clean_jobs flag)
	setDiffMsg    *stratumv1_message.MiningSetDifficulty // recent difficulty message
	extraNonceMsg *stratumv1_message.MiningSetExtranonce // keeps relevant extranonce (picked from mining.subscribe response)
	configureMsg  *stratumv1_message.MiningConfigure
	// TODO: handle pool setExtranonce message

	msgCh              chan stratumv1_message.MiningMessageGeneric // auxillary channel to relay messages
	customPriorityMsgs chan stratumv1_message.MiningMessageGeneric
	isReading          bool       // if false messages will not be availabe to read from outside, used for authentication handshake
	mu                 sync.Mutex // guards isReading

	lastRequestId *atomic.Uint32                 // stratum request id counter
	resHandlers   map[int]StratumV1ResultHandler // allows to register callbacks for particular messages to simplify transaction flow

	log interfaces.ILogger
}

func NewStratumV1Pool(conn net.Conn, log interfaces.ILogger, dest interfaces.IDestination, configureMsg *stratumv1_message.MiningConfigure) *StratumV1PoolConn {
	return &StratumV1PoolConn{

		dest: dest,

		conn:         conn,
		notifyMsgs:   make([]*stratumv1_message.MiningNotify, 100),
		configureMsg: configureMsg,

		msgCh:     make(chan stratumv1_message.MiningMessageGeneric, 100),
		isReading: false, // hold on emitting messages to destination, until handshake

		customPriorityMsgs: make(chan stratumv1_message.MiningMessageGeneric, 100),

		lastRequestId: atomic.NewUint32(0),
		resHandlers:   make(map[int]StratumV1ResultHandler),

		log: log,
	}
}

func (s *StratumV1PoolConn) Run(ctx context.Context) error {
	go func() {
		err := s.run(ctx)
		s.log.Error(err)
	}()
	return nil
}

func (s *StratumV1PoolConn) run(ctx context.Context) error {
	sourceReader := bufio.NewReader(s.conn)
	s.log.Debug("pool reader started")

	for {
		line, isPrefix, err := sourceReader.ReadLine()
		if isPrefix {
			panic("line is too long")
		}

		if err != nil {
			return err
		}

		lib.LogMsg(false, true, s.dest.GetHost(), line, s.log)

		m, err := stratumv1_message.ParseMessageFromPool(line)
		if err != nil {
			s.log.Errorf("Unknown miner message", string(line))
		}

		m = s.readInterceptor(m)

		if s.getIsReading() {
			s.sendToReadCh(m)
		} else {
			s.log.Debugf("pool message was cached but not emitted (%s)", s.GetDest().String())
		}

	}
}

// Allows to connect to a new pool
func (m *StratumV1PoolConn) Connect() error {
	err := m.Run(context.Background())
	if err != nil {
		return err
	}

	if m.configureMsg != nil {
		_, err := m.SendPoolRequestWait(m.configureMsg)
		if err != nil {
			return err
		}
	}

	subscribeRes, err := m.SendPoolRequestWait(stratumv1_message.NewMiningSubscribe(1, "miner", ""))
	if err != nil {
		// TODO: on error fallback to previous pool
		return err
	}
	if subscribeRes.IsError() {
		return fmt.Errorf("invalid subscribe response %s", subscribeRes.Serialize())
	}
	m.log.Debug("connect: subscribe sent")

	extranonce, extranonceSize, err := stratumv1_message.ParseExtranonceSubscribeResult(subscribeRes)
	if err != nil {
		return err
	}

	m.extraNonceMsg = stratumv1_message.NewMiningSetExtranonceV2(extranonce, extranonceSize)

	authMsg := stratumv1_message.NewMiningAuthorize(1, m.dest.Username(), m.dest.Password())
	_, err = m.SendPoolRequestWait(authMsg)
	if err != nil {
		m.log.Debugf("reconnect: error sent subscribe %w", err)

		// TODO: on error fallback to previous pool
		return err
	}
	m.log.Debug("connect: authorize sent")

	m.setIsReading(true)

	return nil
}

// SendPoolRequestWait sends a message and awaits for the response
func (m *StratumV1PoolConn) SendPoolRequestWait(msg stratumv1_message.MiningMessageToPool) (*stratumv1_message.MiningResult, error) {
	id := int(m.lastRequestId.Inc())
	msg.SetID(id)

	err := m.Write(context.TODO(), msg)
	if err != nil {
		return nil, err
	}
	errCh := make(chan error)
	resCh := make(chan stratumv1_message.MiningResult)

	m.registerResultHandler(id, func(a stratumv1_message.MiningResult) stratumv1_message.MiningMessageGeneric {
		if a.IsError() {
			errCh <- errors.New(a.GetError())
		} else {
			resCh <- a
		}
		return nil // do not proxy this request
	})

	select {
	case err := <-errCh:
		return nil, err
	case res := <-resCh:
		return &res, nil
	}
}

func (m *StratumV1PoolConn) registerResultHandler(id int, handler StratumV1ResultHandler) {
	m.resHandlers[id] = handler
}

// Pauses emitting any pool messages, then sends cached messages for a recent job, and then resumes pool message flow
func (m *StratumV1PoolConn) ResendRelevantNotifications(ctx context.Context) {
	m.setIsReading(false)
	defer m.setIsReading(true)
	m.resendRelevantNotifications(ctx)
}

// resendRelevantNotifications sends cached extranonce, set_difficulty and notify messages
// useful after changing miner's destinations
func (m *StratumV1PoolConn) resendRelevantNotifications(ctx context.Context) {
	m.sendToReadCh(m.extraNonceMsg)
	m.log.Infof("extranonce was resent")

	if m.setDiffMsg != nil {
		m.sendToReadCh(m.setDiffMsg)
		m.log.Infof("set-difficulty was resent", m.setDiffMsg)
	}

	for _, msg := range m.notifyMsgs {
		if msg != nil {
			m.sendToReadCh(msg)
			m.log.Infof("notify was resent %s", msg.Serialize())
		}
	}
}

func (s *StratumV1PoolConn) sendToReadCh(msg stratumv1_message.MiningMessageGeneric) {
	timeoutAlert := 30 * time.Second
	for n := 0; true; n++ {
		select {
		case s.msgCh <- msg:
			return
		case <-time.After(timeoutAlert):
			s.log.Errorf("sendToReadCh is blocked for %.1f seconds, pending message %s", timeoutAlert.Seconds()*float64(n), string(msg.Serialize()))
		}
	}
}

// Read reads message from pool
func (s *StratumV1PoolConn) Read() (stratumv1_message.MiningMessageGeneric, error) {
	msg := <-s.msgCh
	return msg, nil
}

// Write writes message to pool
func (m *StratumV1PoolConn) Write(ctx context.Context, msg stratumv1_message.MiningMessageGeneric) error {
	msg = m.writeInterceptor(msg)

	lib.LogMsg(false, false, m.dest.GetHost(), msg.Serialize(), m.log)

	b := fmt.Sprintf("%s\n", msg.Serialize())
	_, err := m.conn.Write([]byte(b))
	return err
}

// Returns current extranonce values
func (m *StratumV1PoolConn) GetExtranonce() (string, int) {
	return m.extraNonceMsg.GetExtranonce()
}

func (m *StratumV1PoolConn) GetDest() interfaces.IDestination {
	return m.dest
}

func (s *StratumV1PoolConn) RemoteAddr() string {
	return s.conn.RemoteAddr().String()
}

// readInterceptor caches relevant messages and invokes callbacks
func (s *StratumV1PoolConn) readInterceptor(m stratumv1_message.MiningMessageGeneric) stratumv1_message.MiningMessageGeneric {
	switch typedMessage := m.(type) {
	case *stratumv1_message.MiningNotify:
		if typedMessage.GetCleanJobs() {
			s.notifyMsgs = s.notifyMsgs[:0]
		}
		s.notifyMsgs = append(s.notifyMsgs, typedMessage.Copy())

	case *stratumv1_message.MiningSetDifficulty:
		s.setDiffMsg = typedMessage.Copy()

	case *stratumv1_message.MiningResult:
		id := typedMessage.GetID()
		handler, ok := s.resHandlers[id]
		if ok {
			handledMsg := handler(*typedMessage.Copy())
			if handledMsg != nil {
				m = handledMsg.(*stratumv1_message.MiningResult)
			}
		}
	}

	return m
}

func (s *StratumV1PoolConn) writeInterceptor(m stratumv1_message.MiningMessageGeneric) stratumv1_message.MiningMessageGeneric {
	switch typedMsg := m.(type) {
	case *stratumv1_message.MiningSubmit:
		typedMsg.SetWorkerName(s.dest.Username())
		m = typedMsg
	}
	return m
}

func (s *StratumV1PoolConn) setIsReading(b bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b {
		s.log.Debugf("reading from pool released (%s)", s.dest)
	} else {
		s.log.Debugf("reading from pool in on hold (%s)", s.dest)
	}
	s.isReading = b
}

func (s *StratumV1PoolConn) getIsReading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isReading
}
