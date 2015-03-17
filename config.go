package rabbit_tracer

import (
	"github.com/BurntSushi/toml"
	"fmt"
	"os"
	"time"
)
type Config struct {
	Rabbit        map[string]server  `toml:"rabbit"`
	Pub     	  map[string]client  `toml:"pub"`
	Sub     	  map[string]client  `toml:"sub"`
	Tracer 		  map[string]logging  `toml:"tracer"`
}

type server struct {
	Uri string
	Exchange string
	Exchangetype  string
	Lifetime time.Duration
	Prefetchcount int
	Prefetchsize int
}
type client struct {
	Queue string
	Bindingkey string
	Consumertag string
	Logfile string
}


type logging struct {
	Logpath string
	Logfilemaxsize int
	Logfilemaxbackup int
	Logfilemaxage int
}


var conf *Config

func init() {
	dir, _ := os.Getwd()
 	
	// order in which to search for config file
	files := []string{
		dir + "/dev.ini",
		dir + "/config.ini",
		dir + "/config/dev.ini",
		dir + "/config/config.ini",
	}

	for _, f := range files {
		if _, err := toml.DecodeFile(f, &conf); err == nil {
			fmt.Printf("Loaded configuration from: %s\n", f)
			break
		} else {		
			fmt.Printf("Erro loading config from: %s\n", err)
		}
	}
}

func GetConfig() *Config {	
	return conf
}