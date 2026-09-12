package log

import (
	"log"
	"os"
)

var logger = log.New(os.Stdout, "", log.LstdFlags)

// F allows kv-style logging: F("http server", "addr", ":8080")
func F(msg string, kv ...any) {
	args := append([]any{msg}, kv...)
	logger.Println(args...)
}

func Infof(format string, args ...any) { logger.Printf(format, args...) }
func Errorf(format string, args ...any) { logger.Printf("ERROR: "+format, args...) }