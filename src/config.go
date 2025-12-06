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
	// 注册当配置文件修改时的处理方法
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
	// 启动时的配置文件检查
	if err = VP.Unmarshal(&CONFIG); err != nil {
		return err
	}
	if err = CONFIG.Check(); err != nil {
		return errors.New("配置文件校验失败: " + err.Error())
	}
	return nil
}

func (c *Config) Check() (err error) {
	// 事先检查所有的配置
	err = c.Server.Check()
	for _, bac := range c.BotApps {
		err = errors.Join(err, bac.Check())
	}
	return
}

type ServerConfig struct {
	Host      string `mapstructure:"host" yaml:"host"`
	Port      uint   `mapstructure:"port" yaml:"port"`
	Suffix    string `mapstructure:"suffix" yaml:"suffix"`
	BotId     string `mapstructure:"bot-id" yaml:"bot-id"`
	UserAgent string `mapstructure:"user-agent" yaml:"user-agent"`
	Default   struct {
		UserId  IdConfig `mapstructure:"user-id" yaml:"user-id"`
		GroupId IdConfig `mapstructure:"group-id" yaml:"group-id"`
	} `mapstructure:"default" yaml:"default"`
	BufferSize int     `mapstructure:"buffer-size" yaml:"buffer-size"`
	SleepTime  float32 `mapstructure:"sleep-time" yaml:"sleep-time"` //重新连接的间隔，单位秒
	Debug      bool    `mapstructure:"debug" yaml:"debug"`
}
type BotAppsConfig struct {
	Name           string        `mapstructure:"name" yaml:"name"`
	Uri            string        `mapstructure:"uri" yaml:"uri"`
	AccessToken    string        `mapstructure:"access-token" yaml:"access-token"`
	UserId         IdConfig      `mapstructure:"user-id" yaml:"user-id"`
	GroupId        IdConfig      `mapstructure:"group-id" yaml:"group-id"`
	PrivateMessage MessageConfig `mapstructure:"private-message" yaml:"private-message"`
	GroupMessage   MessageConfig `mapstructure:"group-message" yaml:"group-message"`
	// 保留顶层 message 以向后兼容历史版本的配置
	//若 private/group 未单独配置 message，则使用此项
	Message MessageConfig `mapstructure:"message" yaml:"message"`
}

type IdConfig struct {
	Mode string  `mapstructure:"mode" yaml:"mode"` // default、whitelist or blacklist
	Ids  []int64 `mapstructure:"ids" yaml:"ids"`
}

type MessageConfig struct {
	Mode          string   `mapstructure:"mode" yaml:"mode"` // on、whitelist or blacklist
	Filters       []string `mapstructure:"filters" yaml:"filters"`
	Prefix        []string `mapstructure:"prefix" yaml:"prefix"`
	PrefixReplace string   `mapstructure:"prefix-replace" yaml:"prefix-replace"`
}

func (sc *ServerConfig) Check() error {

	if sc.Host == "" {
		return errors.New("server.host不能为空")
	}
	if sc.Port == 0 {
		return errors.New("server.port不能为0")
	}
	if sc.BotId == "" {
		return errors.New("server.bot-id不能为空")
	}
	if sc.UserAgent == "" {
		return errors.New("server.user-agent不能为空")
	}
	switch sc.Default.UserId.Mode {
	case "", WHITELIST, BLACKLIST:
		//ok
	default:
		return errors.New("server.default.user-id.mode配置错误，只能是whitelist 或 blacklist")
	}
	switch sc.Default.GroupId.Mode {
	case "", WHITELIST, BLACKLIST:
		//ok
	default:
		return errors.New("server.default.group-id.mode配置错误，只能是whitelist 或 blacklist")
	}
	return nil
}

func (bac *BotAppsConfig) Check() error {
	if bac.Name == "" {
		return errors.New("bot-apps.name不能为空")
	}
	if bac.Uri == "" {
		return fmt.Errorf("%s.uri不能为空", bac.Name)
	}
	// 验证账号黑白名单
	switch bac.UserId.Mode {
	case "", DEFAULT:
		// 使用默认配置
		bac.UserId = CONFIG.Server.Default.UserId
	case WHITELIST, BLACKLIST:
		// ok
	default:
		return fmt.Errorf("%s.user-id.mode配置错误，只能是whitelist或blacklist", bac.Name)
	}
	switch bac.GroupId.Mode {
	case "", DEFAULT:
		bac.GroupId = CONFIG.Server.Default.GroupId
	case WHITELIST, BLACKLIST:
		// ok
	default:
		return fmt.Errorf("%s.group-id.mode配置错误，只能是whitelist或blacklist", bac.Name)
	}
	// 验证消息过滤器
	switch bac.Message.Mode {
	case "", ON, OFF, WHITELIST, BLACKLIST:
		// ok
	default:
		return fmt.Errorf("%s.message.mode配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}
	// 如果private-message.mode为default，则使用message
	switch bac.PrivateMessage.Mode {
	case "", DEFAULT:
		bac.PrivateMessage = bac.Message
	case ON, OFF, WHITELIST, BLACKLIST:
		// ok
	default:
		return fmt.Errorf("%s.private-message.mode配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}
	// 如果group-message.mode为default，则使用message
	switch bac.GroupMessage.Mode {
	case "", DEFAULT:
		bac.GroupMessage = bac.Message
	case ON, OFF, WHITELIST, BLACKLIST:
		// ok
	default:
		return fmt.Errorf("%s.group-message.mode配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}
	return nil
}
