package blockchain

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
	"gitlab.com/TitanInd/hashrouter/lumerinlib"
)

type EthereumWallet struct {
	Account    accounts.Account
	PrivateKey string
}

func NewWallet(mnemonic string, accountIndex int) *EthereumWallet {
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		panic(fmt.Sprintf("Funcname::%s, Fileline::%s, Error::%v", lumerinlib.Funcname(), lumerinlib.FileLine(), err))
	}
	path := hdwallet.MustParseDerivationPath("m/44'/60'/0'/0/" + fmt.Sprint(accountIndex))
	account, err := wallet.Derive(path, false)
	if err != nil {
		panic(fmt.Sprintf("Funcname::%s, Fileline::%s, Error::%v", lumerinlib.Funcname(), lumerinlib.FileLine(), err))
	}
	privateKey, err := wallet.PrivateKeyHex(account)
	if err != nil {
		panic(fmt.Sprintf("Funcname::%s, Fileline::%s, Error::%v", lumerinlib.Funcname(), lumerinlib.FileLine(), err))
	}

	fmt.Println("Contract Manager Account Address:", account)

	return &EthereumWallet{Account: account, PrivateKey: privateKey}
}
