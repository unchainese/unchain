package main

import (
	"bytes"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func getNetworkIp() string {
	allInterfaces, err := net.Interfaces()
	if err != nil {
		log.Println("Error getting network interfaces:", err)
		return ""
	}
	ips := make([]string, 0)
	for _, iface := range allInterfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			log.Println("Error getting addresses for interface:", iface.Name, "error:", err)
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil || ipNet.IP.IsMulticast() || ipNet.IP.IsLinkLocalUnicast() || ipNet.IP.IsLinkLocalMulticast() {
				continue
			}
			if ipNet.IP.IsGlobalUnicast() {
				return ipNet.IP.String()
			}
			ips = append(ips, ipNet.IP.String())
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("0.0.0.0:%d", c.AppPort)
}

func (c Config) SubAddr() string {
	ip := getNetworkIp()
	return fmt.Sprintf("%s:%d", ip, c.AppPort)
}

func osEnvWithDefault(key, def string) string {
	if envVal := strings.TrimSpace(os.Getenv(key)); envVal == "" {
		fmt.Printf("%s defaultValue:  %s\n", key, def)
		return strings.TrimSpace(def)
	} else {
		return envVal
	}
}

func loadEnv() *Config {
	opt := Config{}
	for i := 0; i < reflect.TypeOf(opt).NumField(); i++ {
		propertyName := reflect.TypeOf(opt).Field(i).Name
		key := snakeCaseUpper(propertyName)
		def := reflect.TypeOf(opt).Field(i).Tag.Get("def")
		envOrDefaultValue := osEnvWithDefault(key, def)

		kind := reflect.TypeOf(opt).Field(i).Type.Kind()
		switch kind {
		case reflect.String:
			reflect.ValueOf(&opt).Elem().Field(i).SetString(envOrDefaultValue)
		case reflect.Int, reflect.Int64, reflect.Int32:
			if v, err := strconv.ParseInt(envOrDefaultValue, 10, 64); err == nil {
				reflect.ValueOf(&opt).Elem().Field(i).SetInt(v)
			}
		case reflect.Float32, reflect.Float64:
			if v, err := strconv.ParseFloat(envOrDefaultValue, 64); err == nil {
				reflect.ValueOf(&opt).Elem().Field(i).SetFloat(v)
			}
		case reflect.Uint, reflect.Uint64, reflect.Uint32:
			if v, err := strconv.ParseUint(envOrDefaultValue, 10, 64); err == nil {
				reflect.ValueOf(&opt).Elem().Field(i).SetUint(v)
			}
		case reflect.Bool:
			if v, err := strconv.ParseBool(envOrDefaultValue); err == nil {
				reflect.ValueOf(&opt).Elem().Field(i).SetBool(v)
			}
		default:
			fmt.Printf("unsupported config field type: %s\n", reflect.TypeOf(opt).Field(i).Type.Kind().String())
			continue
		}
	}

	return &opt
}

func snakeCase(camel string) string {
	var buf bytes.Buffer
	for _, c := range camel {
		if 'A' <= c && c <= 'Z' {
			// just convert [A-Z] to _[a-z]
			if buf.Len() > 0 {
				buf.WriteRune('_')
			}
			buf.WriteRune(c - 'A' + 'a')
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func snakeCaseUpper(camel string) string {
	return strings.ToUpper(snakeCase(camel))
}

func (c Config) GetBufferSize() int {
	if c.BufferSize < 1 {
		return 8192
	}
	return c.BufferSize
}

var (
	gitHash   string
	buildTime string
)

var cfg *Config

// Cfg load config from toml file or env
func Cfg() *Config {
	if cfg != nil {
		return cfg
	}
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("failed to load .env file, use env instead")
	}
	cfg = loadEnv()
	cfg.GitHash = gitHash
	cfg.BuildTime = buildTime
	cfg.RunAt = time.Now().Format("2006-01-02 15:04:05")
	return cfg
}

func (c Config) LogLevel() slog.Level {
	l := slog.LevelDebug
	switch strings.ToUpper(c.DebugLevel) {
	case "DEBUG":
		l = slog.LevelDebug
	case "INFO":
		l = slog.LevelInfo
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelError
	}
	return l
}
func (c Config) UserIDS() []string {
	parts := strings.Split(c.AllowUsers, ",")
	ids := make([]string, 0)
	for _, uid := range parts {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			ids = append(ids, uid)
		}
	}
	return ids
}

func (c Config) PushInterval() time.Duration {
	if c.IntervalSecond <= 0 {
		return time.Minute * 60
	}
	return time.Second * time.Duration(c.IntervalSecond)
}
