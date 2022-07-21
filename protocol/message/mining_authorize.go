package message

import "encoding/json"

// Message: {"id": 2, "method": "mining.authorize", "params": ["workername", "password"]}
const MethodMiningAuthorize = "mining.authorize"

type MiningAuthorize struct {
	ID     int                    `json:"id"`
	Method string                 `json:"method,omitempty"`
	Params *miningAuthorizeParams `json:"params"`
}

type miningAuthorizeParams = [2]string

func NewMiningAuthorize() *MiningAuthorize {
	return &MiningAuthorize{
		Method: MethodMiningAuthorize,
		Params: &miningAuthorizeParams{},
	}
}

func ParseMiningAuthorize(b []byte) (*MiningAuthorize, error) {
	m := &MiningAuthorize{}
	return m, json.Unmarshal(b, m)
}

func (m *MiningAuthorize) GetID() int {
	return m.ID
}

func (m *MiningAuthorize) SetID(ID int) {
	m.ID = ID
}

func (m *MiningAuthorize) GetMinerID() string {
	return m.Params[0]
}

func (m *MiningAuthorize) SetMinerID(ID string) {
	m.Params[0] = ID
}

func (m *MiningAuthorize) GetPassword() string {
	return m.Params[1]
}

func (m *MiningAuthorize) SetPassword(pwd string) {
	m.Params[1] = pwd
}

func (m *MiningAuthorize) Serialize() []byte {
	b, _ := json.Marshal(m)
	return b
}

var _ MiningMessageToPool = new(MiningAuthorize)
