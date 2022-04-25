package mining

import (
	"log"
	"os"
	"time"
)

type MiningController struct {
	user     string
	password string
}

func (m *MiningController) ProcessMiningMessage(messageRaw []byte) []byte {

	message, err := CreateMinerMessage(messageRaw)

	if err != nil {
		log.Printf("Failed to process miner message - failed to create model from byte array - %v - %v", string(messageRaw), err)
		return messageRaw
	}

	result, err := message.ProcessMessage(m.user, m.password)

	if err != nil {
		log.Printf("Failed to process miner message - failed to create byte array from model - %v", err)
		return messageRaw
	}

	return result
}

func (m *MiningController) ProcessPoolMessage(messageRaw []byte) []byte {
	return messageRaw
}

func (m *MiningController) SetAuth(user string, password string) {
	m.user = user
	m.password = password
}

func (m *MiningController) Update(message interface{}) {
	m.user = os.Getenv("TEST_POOL_USER")
	m.password = os.Getenv("TEST_POOL_PASSWORD")

	<-time.After(time.Second * 60)

	m.user = os.Getenv("DEFAULT_POOL_USER")
	m.password = os.Getenv("DEFAULT_POOL_PASSWORD")
}
