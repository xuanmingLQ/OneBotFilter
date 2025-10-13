package onebotfilter

import (
	"errors"
	"fmt"
	"log"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// type YamlString string

// // 实现 yaml.Unmarshaler 接口
// func (s *YamlString) UnmarshalYAML(value *yaml.Node) error {
// 	*s = YamlString(value.Value) // 无论是 int 还是 string，这里统一当字符串处理
// 	return nil
// }

type Config struct {
	Server  ServerConfig    `mapstructure:"server" yaml:"server"`
	BotApps []BotAppsConfig `mapstructure:"bot-apps" yaml:"bot-apps"`
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
		if err = CONFIG.Check(); err != nil {
			log.Println("配置文件校验失败:", err)
			return
		}
		err = ReLoadFilters()
		if err != nil {
			log.Println("重新加载过滤器失败:", err)
		}
	})
	if err = VP.Unmarshal(&CONFIG); err != nil {
		return err
	}
	if err = CONFIG.Check(); err != nil {
		return errors.New("配置文件校验失败: " + err.Error())
	}
	return nil
}

func (c *Config) Check() error {
	if c.Server.Host == "" {
		return errors.New("server.host不能为空")
	}
	if c.Server.Port == 0 {
		return errors.New("server.port不能为0")
	}
	if c.Server.BotId == "" {
		return errors.New("server.bot-id不能为空")
	}
	if c.Server.UserAgent == "" {
		return errors.New("server.user-agent不能为空")
	}
	switch c.Server.Default.Private.Mode {
	case "", ON, OFF, WHITELIST, BLACKLIST:
		//ok
	default:
		return errors.New("server.default.private.mode配置错误，只能是on、 off、 whitelist or blacklist")
	}
	switch c.Server.Default.Group.Mode {
	case "", ON, OFF, WHITELIST, BLACKLIST:
		//ok
	default:
		return errors.New("server.default.group.mode配置错误，只能是on、 off、 whitelist or blacklist")
	}
	return nil
}

type ServerConfig struct {
	Host      string `mapstructure:"host" yaml:"host"`
	Port      uint   `mapstructure:"port" yaml:"port"`
	Suffix    string `mapstructure:"suffix" yaml:"suffix"`
	BotId     string `mapstructure:"bot-id" yaml:"bot-id"`
	UserAgent string `mapstructure:"user-agent" yaml:"user-agent"`
	Default   struct {
		Private MessageTypeConfig `mapstructure:"private" yaml:"private"`
		Group   MessageTypeConfig `mapstructure:"group" yaml:"group"`
	} `mapstructure:"default" yaml:"default"`
	SleepTime float32 `mapstructure:"sleep-time" yaml:"sleep-time"` //重新连接的间隔，单位秒
	Debug     bool    `mapstructure:"debug" yaml:"debug"`
}
type BotAppsConfig struct {
	Name        string            `mapstructure:"name" yaml:"name"`
	Uri         string            `mapstructure:"uri" yaml:"uri"`
	AccessToken string            `mapstructure:"access-token" yaml:"access-token"`
	Private     MessageTypeConfig `mapstructure:"private" yaml:"private"`
	Group       MessageTypeConfig `mapstructure:"group" yaml:"group"`
	Message     MessageConfig     `mapstructure:"message" yaml:"message"`
}

type MessageTypeConfig struct {
	Mode string  `mapstructure:"mode" yaml:"mode"` // on、 off、 whitelist or blacklist
	Ids  []int64 `mapstructure:"ids" yaml:"ids"`
}
type MessageConfig struct {
	Mode          string   `mapstructure:"mode" yaml:"mode"` // whitelist or blacklist
	Filters       []string `mapstructure:"filters" yaml:"filters"`
	Prefix        string   `mapstructure:"prefix" yaml:"prefix"`
	PrefixReplace string   `mapstructure:"prefix-replace" yaml:"prefix-replace"`
}

func (bac *BotAppsConfig) Check() error {
	if bac.Name == "" {
		return errors.New("bot-apps.name不能为空")
	}
	if bac.Uri == "" {
		return fmt.Errorf("%s.uri不能为空", bac.Name)
	}
	switch bac.Private.Mode {
	case "", DEFAULT:
		bac.Private = CONFIG.Server.Default.Private
	case ON, OFF, WHITELIST, BLACKLIST:
		//ok
	default:
		return fmt.Errorf("%s.private.mode配置错误，只能是on、 off、 whitelist or blacklist", bac.Name)
	}
	switch bac.Group.Mode {
	case "", DEFAULT:
		bac.Group = CONFIG.Server.Default.Group
	case ON, OFF, WHITELIST, BLACKLIST:
		//ok
	default:
		return fmt.Errorf("%s.group.mode配置错误，只能是on、 off、 whitelist or blacklist", bac.Name)
	}

	switch bac.Message.Mode {
	case "", WHITELIST, BLACKLIST:
		//ok
	default:
		return fmt.Errorf("%s.message.mode配置错误，只能是whitelist or blacklist", bac.Name)
	}
	return nil
}
