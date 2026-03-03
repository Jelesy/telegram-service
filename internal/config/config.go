package config

import (
	"fmt"
	"log"
	"os"
	"telegram-service/internal/printer"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type TelegramConf struct {
	AppId   int    `env:"APP_ID" env-required:""`
	AppHash string `env:"APP_HASH" env-required:""`
}

type ServerConf struct {
	Host string `env:"SERVER_HOST" env-default:"localhost"`
	Port string `env:"SERVER_PORT" env-default:"8080"`
}

type Config struct {
	Env    string `env:"ENV" env-default:"local"`
	Tg     TelegramConf
	Server ServerConf
}

func MustLoad() *Config {
	// Получение текущей рабочей директории
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %s", err)
	}
	printer.CoolSoloLogPrint("Current working directory", cwd)

	// Получение переменных из .env
	err = godotenv.Load()
	if err != nil {
		log.Fatalf("Failed to load .env file: %s", err)
	}

	// Заполнение структуры
	var conf Config
	if err := cleanenv.ReadEnv(&conf); err != nil {
		log.Fatalf("can't read config: %s", err)
	}
	printer.CoolSoloLogPrint("Config", conf)

	return &conf
}

func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}
