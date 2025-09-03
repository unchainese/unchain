package main

import (
	"log"
	"log/slog"
	"os"
)

func SetupLogger(c *Config) (fd *os.File) {
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
	slog.SetLogLoggerLevel(c.LogLevel())
	fd = os.Stdout
	if c.LogFile != "" {
		file, err := os.OpenFile(c.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Println("Failed to open log file:", err, c.LogFile)
		} else {
			fd = file
		}
	}
	log.SetOutput(fd)
	return fd
}
