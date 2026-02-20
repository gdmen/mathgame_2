package common

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type Config struct {
	MySQLHost              string `json:"mysql_host"`
	MySQLPort              string `json:"mysql_port"`
	MySQLDatabase          string `json:"mysql_database"`
	MySQLUser              string `json:"mysql_user"`
	MySQLPass              string `json:"mysql_pass"`
	ApiHost                string `json:"api_host"`
	ApiPort                string `json:"api_port"`
	Auth0Domain            string `json:"auth0_domain"`
	Auth0ClientId          string `json:"auth0_clientId"`
	Auth0Audience          string `json:"auth0_audience"`
	OpenAiApiKey           string `json:"openai_api_key"`
	YouTubeAPIKey          string `json:"youtube_api_key"`
	EventReportingInterval int    `json:"event_reporting_interval"`
	DebugQuickplay         bool   `json:"debug_quickplay"`
}

func ReadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	c := &Config{}
	if err := decoder.Decode(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate returns an error if any required config string field is unset (empty or whitespace).
func (c *Config) Validate() error {
	var missing []string
	t := reflect.TypeOf(*c)
	v := reflect.ValueOf(*c)
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type.Kind() != reflect.String {
			continue
		}
		name := t.Field(i).Tag.Get("json")
		if name == "" {
			name = t.Field(i).Name
		}
		if strings.TrimSpace(v.Field(i).String()) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config missing required values: %s", strings.Join(missing, ", "))
	}
	return nil
}
