package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config 配置
type Config struct {
	Zaplog []LogConfig `yaml:"zaplog"`
}

// LogConfig 日志实例配置
type LogConfig struct {
	Name        string `yaml:"name" mapstructure:"name"`                 // 日志名称
	Level       string `yaml:"level" mapstructure:"level"`               // 日志级别
	FileName    string `yaml:"file_name" mapstructure:"file_name"`       // 日志文件路径
	MaxAge      int    `yaml:"max_age" mapstructure:"max_age"`           // 最大保存天数
	MaxSize     int    `yaml:"max_size" mapstructure:"max_size"`         // 单个文件最大大小（MB）
	MaxBackups  int    `yaml:"max_backups" mapstructure:"max_backups"`   // 最大备份数量
	Compress    bool   `yaml:"compress" mapstructure:"compress"`         // 是否压缩
	JsonEncoder bool   `yaml:"json_encoder" mapstructure:"json_encoder"` // 是否使用 JSON 格式
	Development bool   `yaml:"development" mapstructure:"development"`   // 开发模式
	ShowCaller  bool   `yaml:"show_caller" mapstructure:"show_caller"`   // 是否显示调用者信息
}

var (
	loggers = make(map[string]*zap.Logger)
	metux   sync.RWMutex
)

func validateConfig(cfg *Config) error {
	if len(cfg.Zaplog) == 0 {
		return fmt.Errorf("no logger configurations found")
	}
	var hasDefault bool = false
	for _, lc := range cfg.Zaplog {
		if lc.Name == "" {
			return fmt.Errorf("logger name is required")
		}
		if lc.Name == "default" {
			hasDefault = true
		}
		if lc.FileName == "" {
			return fmt.Errorf("logger %s: file_name is required", lc.Name)
		}
	}
	if !hasDefault {
		return fmt.Errorf("no default logger configuration found")
	}
	return nil
}

func setDefault(cfg *LogConfig) {
	if cfg.Level == "" ||
		(cfg.Level != "debug" && cfg.Level != "info" && cfg.Level != "warn" &&
			cfg.Level != "error" && cfg.Level != "panic" && cfg.Level != "fatal") {
		cfg.Level = "info"
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 7
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 10
	}
}

// LoadConfig 加载配置
func LoadConfig(configPath string) (Config, error) {
	var cfg Config

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}

	//fmt.Printf("config file content: %v", v.AllSettings())

	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	/* 	data, err := os.ReadFile(configPath)
	   	if err != nil {
	   		return cfg, err
	   	}
	   	err = json.Unmarshal(data, &cfg)
	   	if err != nil {
	   		return cfg, err
	   	} */

	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func getEncoder(jsonFormat bool) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	if jsonFormat {
		return zapcore.NewJSONEncoder(encoderConfig)
	}

	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func getLevel(level string) zapcore.Level {
	level = strings.ToLower(level)
	switch level {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "panic":
		return zap.PanicLevel
	case "fatal":
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}

func getWriteSyncer(cfg LogConfig) zapcore.WriteSyncer {
	if err := os.MkdirAll(filepath.Dir(cfg.FileName), 0755); err != nil {
		panic(err)
	}

	lumberjackLogger := &lumberjack.Logger{
		Filename:   cfg.FileName,
		MaxAge:     cfg.MaxAge,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	return zapcore.NewMultiWriteSyncer(
		zapcore.AddSync(lumberjackLogger),
		zapcore.AddSync(os.Stdout),
	)
}

func newLogger(cfg LogConfig) (*zap.Logger, error) {
	setDefault(&cfg)

	encoder := getEncoder(cfg.JsonEncoder)

	core := zapcore.NewCore(
		encoder,
		getWriteSyncer(cfg),
		getLevel(cfg.Level),
	)

	options := []zap.Option{}
	if cfg.ShowCaller {
		options = append(options, zap.AddCaller())
	}
	if cfg.Development {
		options = append(options, zap.Development())
	}

	return zap.New(core, options...), nil
}

// InitFromLocalFileConfig 初始化日志
func InitFromLocalFileConfig(configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	metux.Lock()
	defer metux.Unlock()

	for _, lc := range cfg.Zaplog {
		logger, err := newLogger(lc)
		if err != nil {
			return fmt.Errorf("failed to create logger %s: %w", lc.Name, err)

		}
		loggers[lc.Name] = logger

		if lc.Name == "default" {
			zap.ReplaceGlobals(logger)
		}
	}
	return nil
}

// GetLogger 获取指定名称的logger，如果不存在，则返回全局Default logger
func GetLogger(name string) *zap.Logger {
	metux.RLock()
	defer metux.RUnlock()

	logger, ok := loggers[name]
	if !ok || name == "default" {
		logger = zap.L()
	}
	return logger
}

// GetDefaultLogger 返回全局Default logger
func GetDefaultLogger() *zap.Logger {
	return GetLogger("default")
}

// Close 关闭所有的logger
func Close() {
	metux.Lock()
	defer metux.Unlock()

	for name, logger := range loggers {
		_ = logger.Sync()
		delete(loggers, name)
	}
}
