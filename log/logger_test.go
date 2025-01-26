package log

import (
	"fmt"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestLoadConfig(t *testing.T) {
	err := InitFromLocalFileConfig("./log_config.yaml")
	if err != nil {
		panic(err)
	}
	defaultLog := GetDefaultLogger()
	defaultLog.Debug("aa", zapcore.Field{Key: "firstname", String: "chen", Type: zapcore.StringType}, zapcore.Field{Key: "age", Integer: 40, Type: zapcore.Int32Type})

	errorLog := GetLogger("error")
	errorLog.Error("bb", zapcore.Field{Key: "error", Interface: fmt.Errorf("divided by zero"), Type: zapcore.ErrorType})
}
