package onebotfilter

import (
	"errors"
	"fmt"
	"log"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

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
		Private struct {
			Mode string  `mapstructure:"mode" yaml:"mode"` // on、 off、 whitelist or blacklist
			Ids  []int64 `mapstructure:"ids" yaml:"ids"`
		} `mapstructure:"private" yaml:"private"`
		Group struct {
			Mode string  `mapstructure:"mode" yaml:"mode"` // on、 off、 whitelist or blacklist
			Ids  []int64 `mapstructure:"ids" yaml:"ids"`
		} `mapstructure:"group" yaml:"group"`
	} `mapstructure:"default" yaml:"default"`
	BufferSize int     `mapstructure:"buffer-size" yaml:"buffer-size"`
	SleepTime  float32 `mapstructure:"sleep-time" yaml:"sleep-time"` //重新连接的间隔，单位秒
	Debug      bool    `mapstructure:"debug" yaml:"debug"`
}
type BotAppsConfig struct {
	Name        string            `mapstructure:"name" yaml:"name"`
	Uri         string            `mapstructure:"uri" yaml:"uri"`
	AccessToken string            `mapstructure:"access-token" yaml:"access-token"`
	Private     MessageTypeConfig `mapstructure:"private" yaml:"private"`
	Group       MessageTypeConfig `mapstructure:"group" yaml:"group"`
	// 保留顶层 message 以向后兼容历史版本的配置
	//若 private/group 未单独配置 message，则使用此项
	Message MessageContentConfig `mapstructure:"message" yaml:"message"`
}

type MessageTypeConfig struct {
	Mode    string               `mapstructure:"mode" yaml:"mode"` // default、on、 off、 whitelist or blacklist
	Ids     []int64              `mapstructure:"ids" yaml:"ids"`
	Message MessageContentConfig `mapstructure:"message" yaml:"message"` // 在 private/group 下允许单独配置 message
}
type MessageContentConfig struct {
	Mode          string   `mapstructure:"mode" yaml:"mode"` // on、whitelist or blacklist
	Filters       []string `mapstructure:"filters" yaml:"filters"`
	Prefix        []string `mapstructure:"prefix" yaml:"prefix"`
	PrefixReplace string   `mapstructure:"prefix-replace" yaml:"prefix-replace"`
}

func (bac *BotAppsConfig) Check() error {
	if bac.Name == "" {
		return errors.New("bot-apps.name不能为空")
	}
	if bac.Uri == "" {
		return fmt.Errorf("%s.uri不能为空", bac.Name)
	}

	// 若未配置 private.mode，则仅继承默认的 Mode和ids
	// 仅在 mode 为空或为 DEFAULT 时设置 Mode和ids，避免message配置被覆盖
	switch bac.Private.Mode {
	case "", DEFAULT:
		// 只继承 mode 和 ids 字段，不覆盖 message
		bac.Private.Mode = CONFIG.Server.Default.Private.Mode
		bac.Private.Ids = CONFIG.Server.Default.Private.Ids
	case ON, OFF, WHITELIST, BLACKLIST:
		//ok
	default:
		return fmt.Errorf("%s.private.mode配置错误，只能是on、 off、 whitelist or blacklist", bac.Name)
	}

	// 同理地处理 group 的 mode 和 group.ids ，不覆盖 group.message
	switch bac.Group.Mode {
	case "", DEFAULT:
		bac.Group.Mode = CONFIG.Server.Default.Group.Mode
		bac.Group.Ids = CONFIG.Server.Default.Group.Ids
	case ON, OFF, WHITELIST, BLACKLIST:
		//ok 单独设置
	default:
		return fmt.Errorf("%s.group.mode配置错误，只能是on、 off、 whitelist or blacklist", bac.Name)
	}

	// 验证 message.（默认 message）与 private/group 下的 message（如果存在的话）
	// 默认 message
	switch bac.Message.Mode {
	case "", ON, WHITELIST, BLACKLIST:
		//ok
	default:
		return fmt.Errorf("%s.message.mode配置错误，只能是 on、whitelist 或 blacklist", bac.Name)
	}
	// private.message (如果被设置，验证 mode)
	switch bac.Private.Message.Mode {
	case "", DEFAULT:
		bac.Private.Message = bac.Message
	case ON, WHITELIST, BLACKLIST:
		// ok 单独设置
	default:
		return fmt.Errorf("%s.private.message.mode配置错误，只能是 default、on、whitelist 或 blacklist", bac.Name)
	}
	// group.message (如果被设置，验证 mode)
	switch bac.Group.Message.Mode {
	case "", DEFAULT:
		bac.Group.Message = bac.Message
	case ON, WHITELIST, BLACKLIST:
		// ok 单独设置
	default:
		return fmt.Errorf("%s.group.message.mode配置错误，只能是 default、on、whitelist 或 blacklist", bac.Name)
	}
	return nil
}
