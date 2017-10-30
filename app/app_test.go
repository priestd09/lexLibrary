// Copyright (c) 2017 Townsourced Inc.

package app_test

import (
	"flag"
	"log"
	"os"
	"testing"

	"github.com/lexLibrary/lexLibrary/data"
	"github.com/lexLibrary/lexLibrary/web"
	"github.com/spf13/viper"
)

var flagConfigFile string

func TestMain(m *testing.M) {
	flag.StringVar(&flagConfigFile, "config", "./config.yaml", "Sets the path to the configuration file. Either a .YAML, .JSON, or .TOML file")

	flag.Parse()
	cfg := struct {
		Web  web.Config
		Data data.Config
	}{
		Web:  web.Config{},
		Data: data.Config{},
	}

	viper.SetConfigFile(flagConfigFile)

	err := viper.ReadInConfig()
	if err != nil {

		if os.IsNotExist(err) {
			// open sqlite db in memory for testing
			cfg.Data = data.Config{
				DatabaseType:       "sqlite",
				DatabaseURL:        "file::memory:?mode=memory&cache=shared",
				MaxIdleConnections: 1,
				MaxOpenConnections: 1,
			}
		} else {
			log.Fatal(err)
		}
	} else {
		viper.Unmarshal(&cfg)
	}

	err = data.Init(cfg.Data)
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(m.Run())
}
