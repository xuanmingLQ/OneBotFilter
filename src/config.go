package onebotfilter

import (
	"log"
	"os"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	VP     *viper.Viper
	CONFIG Config
)

type YamlString string

// 实现 yaml.Unmarshaler 接口
func (s *YamlString) UnmarshalYAML(value *yaml.Node) error {
	*s = YamlString(value.Value) // 无论是 int 还是 string，这里统一当字符串处理
	return nil
}

type Config struct {
	Server    ServerConfig   `mapstructure:"server" yaml:"server"`
	Whitelist []ClientConfig `mapstructure:"whitelist" yaml:"whitelist"`
	Blacklist []ClientConfig `mapstructure:"blacklist" yaml:"blacklist"`
}

func LoadConfigVP(path string) error {
	VP = viper.New()
	VP.SetConfigFile(path)
	VP.SetConfigType("yaml")
	err := VP.ReadInConfig()
	if err != nil {
		return err
	}
	VP.WatchConfig()
	VP.OnConfigChange(func(e fsnotify.Event) {
		log.Println("config file changed:", e.Name)
		if err = VP.Unmarshal(&CONFIG); err != nil {
			log.Println(err)
		}
		ReLoadFilters()
	})
	if err = VP.Unmarshal(&CONFIG); err != nil {
		return err
	}
	return nil
}
func LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &CONFIG); err != nil {
		return err
	}
	return nil
}

type ServerConfig struct {
	Host      string     `mapstructure:"host" yaml:"host"`
	Port      uint       `mapstructure:"port" yaml:"port"`
	Suffix    string     `mapstructure:"suffix" yaml:"suffix"`
	BotId     YamlString `mapstructure:"bot-id" yaml:"bot-id"`
	UserAgent string     `mapstructure:"user-agent" yaml:"user-agent"`
	Debug     bool       `mapstructure:"debug" yaml:"debug"`
}
type ClientConfig struct {
	Name        string   `mapstructure:"name" yaml:"name"`
	Uri         string   `mapstructure:"uri" yaml:"uri"`
	AccessToken string   `mapstructure:"access-token" yaml:"access-token"`
	Filters     []string `mapstructure:"filters" yaml:"filters"`
	Prefix      string   `mapstructure:"prefix" yaml:"prefix"`
}
