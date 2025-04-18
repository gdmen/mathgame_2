package common

import (
	"encoding/json"
	"os"
)

type Config struct {
	MySQLHost     string `json:"mysql_host"`
	MySQLPort     string `json:"mysql_port"`
	MySQLDatabase string `json:"mysql_database"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPass     string `json:"mysql_pass"`
	ApiHost       string `json:"api_host"`
	ApiPort       string `json:"api_port"`
	Auth0Domain   string `json:"auth0_domain"`
	Auth0Audience string `json:"auth0_audience"`
	OpenAiApiKey  string `json:"openai_api_key"`
	Debug         bool   `json:"debug"`
}

func ReadConfig(path string) (*Config, error) {
	file, _ := os.Open(path)
	decoder := json.NewDecoder(file)
	c := &Config{}
	err := decoder.Decode(c)
	return c, err
}
