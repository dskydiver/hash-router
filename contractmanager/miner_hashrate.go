package contractmanager

import "gitlab.com/TitanInd/hashrouter/miner"

type HashrateListItem struct {
	Hashrate      int
	TotalHashrate int
	MinerID       string
	SplitPtr      *miner.Split
}

func (m HashrateListItem) GetHashrateGHS() int {
	return m.Hashrate
}

func (m HashrateListItem) SetHashrateGHS(hashrate int) {
	m.Hashrate = hashrate
}

func (m HashrateListItem) GetTotalHashrateGHS() int {
	return m.TotalHashrate

}

func (m HashrateListItem) GetSourceID() string {
	return m.MinerID
}

func (m HashrateListItem) GetPercentage() float64 {
	return float64(m.Hashrate) / float64(m.TotalHashrate)
}

type HashrateList []HashrateListItem

func (m HashrateList) Len() int      { return len(m) }
func (m HashrateList) Swap(i, j int) { m[i], m[j] = m[j], m[i] }
func (m HashrateList) Less(i, j int) bool {
	return m[i].Hashrate < m[j].Hashrate
}
