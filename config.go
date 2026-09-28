package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

type config struct {
	publicMemberID string
	publicPassHash string
	publicUser     string
	publicPassword string
	rootScheme     string
	rootHost       string
	rootPath       string
	connTimeout    time.Duration
	connKeepAlive  time.Duration
	connMaxIdle    int
	respTimeout    time.Duration
}

var conf config

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %q not set", key)
	}
	return v
}

func envStringDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("env %q: invalid int %q: %v", key, v, err)
	}
	return n
}

// значение в env задаётся в секундах, как и в оригинале
func envSecondsDefault(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("env %q: invalid seconds %q: %v", key, v, err)
	}
	return time.Duration(n) * time.Second
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("generate random value: %v", err)
	}
	return hex.EncodeToString(b)
}

func loadConfig() {
	conf.publicMemberID = mustEnv("PANDA_PUBLIC_MEMBER_ID")
	conf.publicPassHash = mustEnv("PANDA_PUBLIC_PASS_HASH")
	// без явно заданных кредов публичный доступ закрыт случайным логином/паролем
	conf.publicUser = os.Getenv("PANDA_PUBLIC_USER")
	conf.publicPassword = os.Getenv("PANDA_PUBLIC_PASSWORD")
	if conf.publicUser == "" || conf.publicPassword == "" {
		if conf.publicUser == "" {
			conf.publicUser = "u" + randomHex(8)
		}
		if conf.publicPassword == "" {
			conf.publicPassword = randomHex(16)
		}
		log.Printf("PANDA_PUBLIC_USER/PANDA_PUBLIC_PASSWORD not set, generated public credentials: %s:%s",
			conf.publicUser, conf.publicPassword)
	}
	conf.rootScheme = envStringDefault("PANDA_ROOT_SCHEME", "https")
	conf.rootHost = os.Getenv("PANDA_ROOT_HOST")
	if conf.rootHost != "" {
		conf.rootPath = fmt.Sprintf("%s://%s", conf.rootScheme, conf.rootHost)
	} else {
		conf.rootPath = ""
	}
	conf.connTimeout = envSecondsDefault("PANDA_CONN_TIMEOUT", 30*time.Second)
	conf.connKeepAlive = envSecondsDefault("PANDA_CONN_KEEP_ALIVE", 30*time.Second)
	conf.connMaxIdle = envIntDefault("PANDA_CONN_MAX_IDLE", 16)
	conf.respTimeout = envSecondsDefault("PANDA_RESPONSE_TIMEOUT", 60*time.Second)
}
