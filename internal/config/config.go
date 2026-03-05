package config

import (
	"fmt"
	"log"
	"os"
	"telegram-service/internal/colorlog"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	envProd  = "prod"
	envTest  = "test"
	envLocal = "local"
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
	Env string `env:"ENV" env-default:"local"`
	TelegramConf
	ServerConf
}

// MustLoad load environment variables from .env files.
//
// `Filenames` is env variables paths (optional)
func MustLoad(filenames ...string) *Config {
	// Получение текущей рабочей директории
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %s", err)
	}
	colorlog.Solo("Current working directory", cwd)

	// Получение переменных из .env
	err = godotenv.Load(filenames...)
	if err != nil {
		log.Fatalf("Failed to load .env file: %s", err)
	}

	// Заполнение структуры
	var conf Config
	if err := cleanenv.ReadEnv(&conf); err != nil {
		log.Fatalf("can't read config: %s", err)
	}
	colorlog.Solo("Config", conf)

	return &conf
}

// GetAddress return full address, for example [127.0.0.1:8080]
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// ConfigureGrpcServer configure grpc-server using config
func (c *Config) ConfigureGrpcServer(server *grpc.Server) {
	// reflection for local or test
	if c.Env == envLocal || c.Env == envTest {
		reflection.Register(server)
	}
	// ...
}
