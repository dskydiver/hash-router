package constants

import (
	"time"
)

const (
	ContAvailableState string = "AvailableState"
	ContRunningState   string = "RunningState"
)

type CloseoutType uint8

const (
	CloseoutTypeCancel       CloseoutType = 0 // to be triggered by the buyer or validator if a contract needs to be canceled early for any reason
	CloseoutTypeOnlyWithdraw CloseoutType = 1 // to be triggered by the seller to withdraw funds at any time during the smart contracts lifecycle (contract is not closing)
	CloseoutTypeWithoutClaim CloseoutType = 2 // closeout without claiming funds
	CloseoutTypeWithClaim    CloseoutType = 3 // closeout with claiming funds
)

const (
	ValidationBufferPeriod time.Duration = 20 * time.Minute // buffer period before buyer node starts validating newly running contract
)
