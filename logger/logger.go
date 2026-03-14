package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

func Init(env string) {
	var err error 

	if env == "production" {
		Log, err = zap.NewProduction()
	} else {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		Log, err = config.Build()
	}

	if err != nil {
		log.Fatal("Could not initialize logger: ", err)
	}

	Log.Info("Logger initialised", zap.String("env", env))
}

func Sync() {
	_ = Log.Sync()
}
