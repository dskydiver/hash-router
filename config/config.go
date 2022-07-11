package config

// Validation tags described here: https://github.com/go-playground/validator
type Config struct {
	Web struct {
		Address string `env:"WEB_ADDRESS" flag:"web-address" desc:"http server address host:port" validate:"required,hostname_port"`
	}
	Pool struct {
		Address  string `env:"POOL_ADDRESS" flag:"pool-address" validate:"required,hostname_port"`
		User     string `env:"POOL_USER" flag:"pool-user" validate:"required"`
		Password string `env:"POOL_PASSWORD" flag:"pool-password"`
	}
	Contract struct {
		Address string `env:"CONTRACT_ADDRESS" flag:"contract-address" validate:"required,eth_addr"`
	}
	EthNode struct {
		Address string `env:"ETH_NODE_ADDRESS" flag:"eth-node-address" validate:"required,url"`
	}
	Log struct {
		Syslog bool `env:"LOG_SYSLOG" flag:"log-syslog"`
	}
}
