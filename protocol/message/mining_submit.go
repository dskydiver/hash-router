package message

import "encoding/json"

// Message: {"id": 4, "method": "mining.submit", "params": ["shev8.local", "620daf25f", "0000000000000000", "62cea7a6", "f9b40000"]}
const MethodMiningSubmit = "mining.submit"

type MiningSubmit struct {
	ID     int      `json:"id"`
	Method string   `json:"method,omitempty"`
	Params []string `json:"params"` // worker_name, job_id, extranonce2, ntime, nonce and optional version_bits (BIP_0310)
}

func ParseMiningSubmit(b []byte) (*MiningSubmit, error) {
	m := &MiningSubmit{}
	return m, json.Unmarshal(b, m)
}

func (m *MiningSubmit) GetID() int {
	return m.ID
}

func (m *MiningSubmit) SetID(ID int) {
	m.ID = ID
}

func (m *MiningSubmit) GetWorkerName() string {
	return m.Params[0]
}

func (m *MiningSubmit) SetWorkerName(name string) {
	m.Params[0] = name
}

func (m *MiningSubmit) Serialize() []byte {
	b, _ := json.Marshal(m)
	return b
}

var _ MiningMessageToPool = new(MiningSubmit)
