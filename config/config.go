package config

import (
	"log"

	"github.com/spf13/viper"
)


type Config struct {
	AppName string 
	AppEnv string 
	AppPort string 

	DBHost string 
	DBPort string 
	DBUser string 
	DBPassword string 
	DBName string 

	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTAccessExpiry  int
	JWTRefreshExpiry int

	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string

	RedisURL string
}

func Load() *Config {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Failed to read env file: ", err)
	}

	return &Config{
		AppName: viper.GetString("APP_NAME"),
		AppPort: viper.GetString("APP_PORT"),
		AppEnv:  viper.GetString("APP_ENV"),

		DBHost:     viper.GetString("DB_HOST"),
		DBPort:     viper.GetString("DB_PORT"),
		DBUser:     viper.GetString("DB_USER"),
		DBPassword: viper.GetString("DB_PASSWORD"),
		DBName:     viper.GetString("DB_NAME"),

		JWTAccessSecret:  viper.GetString("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: viper.GetString("JWT_REFRESH_SECRET"),
		JWTAccessExpiry:  viper.GetInt("JWT_ACCESS_EXPIRY"),
		JWTRefreshExpiry: viper.GetInt("JWT_REFRESH_EXPIRY"),

		R2AccountID:       viper.GetString("R2_ACCOUNT_ID"),
		R2AccessKeyID:     viper.GetString("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: viper.GetString("R2_SECRET_ACCESS_KEY"),
		R2BucketName:      viper.GetString("R2_BUCKET_NAME"),
		R2PublicURL:       viper.GetString("R2_PUBLIC_URL"),

		RedisURL: viper.GetString("REDIS_URL"),
	}
}