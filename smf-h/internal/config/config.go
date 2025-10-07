package config

import (
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Root 顶级配置结构
type Root struct {
	Server ServerConfig  `yaml:"server"`
	MySQL  MySQLConfig   `yaml:"mysql"`
	Chat   ChatConfig    `yaml:"chat"`
	Auth   AuthConfig    `yaml:"auth"`
	Models []ModelPolicy `yaml:"models"`
	Redis  RedisConfig   `yaml:"redis"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug / release
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Charset  string `yaml:"charset"`
}

type ChatConfig struct {
	DefaultModel    string        `yaml:"default_model"`
	MaxHistory      int           `yaml:"max_history"`
	MemoryLimitMsgs int           `yaml:"memory_limit_msgs"`
	RequestTimeout  time.Duration `yaml:"request_timeout"` // e.g. 10s
}

type AuthConfig struct {
	JWTSecret string        `yaml:"jwt_secret"`
	AccessTTL time.Duration `yaml:"access_ttl"` // 例如 24h / 12h
}

// RedisConfig Redis 连接配置
type RedisConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	PoolSize     int           `yaml:"pool_size"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// ModelPolicy 模型权限策略：id + min_level
type ModelPolicy struct {
	ID       string `yaml:"id" json:"id"`
	MinLevel int    `yaml:"min_level" json:"min_level"`
}

// Load 读取 YAML 文件并解析
func Load(path string) (Root, error) {
	var cfg Root
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	ApplyEnvOverrides(&cfg)
	return cfg, nil
}

// ApplyEnvOverrides 允许使用环境变量安全覆写敏感或动态配置（如部署 / CI 注入）
// 仅当对应环境变量非空时才覆盖。
func ApplyEnvOverrides(cfg *Root) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("SERVER_MODE"); v != "" {
		cfg.Server.Mode = v
	}

	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfg.MySQL.Host = v
	}
	if v := os.Getenv("MYSQL_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.MySQL.Port = p
		}
	}
	if v := os.Getenv("MYSQL_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.MySQL.Password = v
	}
	if v := os.Getenv("MYSQL_DB"); v != "" {
		cfg.MySQL.Database = v
	}
	if v := os.Getenv("MYSQL_CHARSET"); v != "" {
		cfg.MySQL.Charset = v
	}

	if v := os.Getenv("AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("AUTH_ACCESS_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Auth.AccessTTL = d
		}
	}

	if v := os.Getenv("CHAT_DEFAULT_MODEL"); v != "" {
		cfg.Chat.DefaultModel = v
	}
	if v := os.Getenv("CHAT_MAX_HISTORY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Chat.MaxHistory = n
		}
	}
	if v := os.Getenv("CHAT_MEMORY_LIMIT_MSGS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Chat.MemoryLimitMsgs = n
		}
	}
	if v := os.Getenv("CHAT_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Chat.RequestTimeout = d
		}
	}

	if v := os.Getenv("REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("REDIS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Redis.Port = p
		}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = n
		}
	}
	if v := os.Getenv("REDIS_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.PoolSize = n
		}
	}
	if v := os.Getenv("REDIS_DIAL_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Redis.DialTimeout = d
		}
	}
	if v := os.Getenv("REDIS_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Redis.ReadTimeout = d
		}
	}
	if v := os.Getenv("REDIS_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Redis.WriteTimeout = d
		}
	}
}
