package stratumv1_message

import (
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/TitanInd/hashrouter/lib"
)

var (
	ErrStratumV1Unmarshal = errors.New("cannot unmarshal stratumv1 message")
	ErrStratumV1Unknown   = errors.New("unknown stratumv1 message")
)

func ParseMessageToPool(raw []byte) (MiningMessageToPool, error) {
	msg := &MiningGeneric{}
	err := json.Unmarshal(raw, msg)
	if err != nil {
		return nil, lib.WrapError(ErrStratumV1Unmarshal, err)
	}

	switch msg.Method {
	case MethodMiningSubscribe:
		return ParseMiningSubscribe(raw)

	case MethodMiningAuthorize:
		return ParseMiningAuthorize(raw)

	case MethodMiningSubmit:
		return ParseMiningSubmit(raw)

	case MethodMiningMultiVersion:
		return ParseMiningMultiVersion(raw)

	default:
		return nil, lib.WrapError(fmt.Errorf("unknown message to pool: %s", raw), ErrStratumV1Unknown)
	}
}

func ParseMessageFromPool(raw []byte) (MiningMessageGeneric, error) {
	msg := &MiningGeneric{}

	err := json.Unmarshal(raw, msg)
	if err != nil {
		return nil, lib.WrapError(ErrStratumV1Unmarshal, err)
	}

	if msg.Method == MethodMiningNotify {
		return ParseMiningNotify(raw)
	}
	if msg.Method == MethodMiningSetDifficulty {
		return ParseMiningSetDifficulty(raw)
	}
	if msg.Result != nil {
		return ParseMiningResult(raw)
	}

	return nil, lib.WrapError(fmt.Errorf("unknown message from pool: %s", raw), ErrStratumV1Unknown)
}
