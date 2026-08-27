package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

func LoadDotenv(filename string) error {
	values, err := godotenv.Read(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}
