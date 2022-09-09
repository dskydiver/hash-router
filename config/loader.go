package config

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/omeid/uconfig/flat"
	"gitlab.com/TitanInd/hashrouter/lib"
)

const (
	TagEnv  = "env"
	TagFlag = "flag"
	TagDesc = "desc"
)

var (
	ErrEnvLoad          = errors.New("error during loading .env file")
	ErrEnvParse         = errors.New("cannot parse env variable")
	ErrFlagParse        = errors.New("cannot parse flag")
	ErrConfigInvalid    = errors.New("invalid config struct")
	ErrConfigValidation = errors.New("config validation error")
)

func LoadConfig(cfg interface{}, osArgs *[]string) error {
	err := godotenv.Load(".env")
	if err != nil {
		return lib.WrapError(ErrEnvLoad, err)
	}

	// recursively iterates over each field of the nested struct
	fields, err := flat.View(cfg)
	if err != nil {
		return lib.WrapError(ErrConfigInvalid, err)
	}

	flagset := flag.NewFlagSet("", flag.ContinueOnError)

	for _, field := range fields {
		envName, ok := field.Tag(TagEnv)
		if !ok {
			continue
		}

		envValue := os.Getenv(envName)
		err = field.Set(envValue)
		if err != nil {
			return lib.WrapError(ErrEnvParse, fmt.Errorf("%s: %w", envName, err))
		}

		flagName, ok := field.Tag(TagFlag)
		if !ok {
			continue
		}

		flagDesc, _ := field.Tag(TagDesc)

		// writes flag value to variable
		flagset.Var(field, flagName, flagDesc)
	}

	var args []string
	if osArgs != nil {
		args = *osArgs
	} else {
		args = os.Args
	}

	// flags override .env variables
	err = flagset.Parse(args[1:])
	if err != nil {
		return lib.WrapError(ErrFlagParse, err)
	}

	err = validator.New().Struct(cfg)
	if err != nil {
		return lib.WrapError(ErrConfigValidation, err)
	}

	return nil
}
