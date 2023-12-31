package lib

import (
	"net/url"

	"gitlab.com/TitanInd/hashrouter/interfaces"
)

type Dest struct {
	url url.URL
}

func ParseDest(uri string) (Dest, error) {
	res, err := url.Parse(uri)
	if err != nil {
		return Dest{}, err
	}
	res.Scheme = "" // drop stratum+tcp prefix to avoid comparison issues
	return Dest{*res}, nil
}

func (v Dest) Username() string {
	return v.url.User.Username()
}

func (v Dest) Password() string {
	pwd, _ := v.url.User.Password()
	return pwd
}

func (v Dest) GetHost() string {
	return v.url.Host
}

func (v Dest) IsEqual(target interfaces.IDestination) bool {
	return v.String() == target.String()
}

func (v Dest) String() string {
	return v.url.String()
}
