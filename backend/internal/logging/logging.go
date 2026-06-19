package logging

import "log"

func Info(tag string, format string, args ...any) {
	log.Printf("[%s] "+format, append([]any{tag}, args...)...)
}

func Error(tag string, format string, args ...any) {
	log.Printf("[ERROR][%s] "+format, append([]any{tag}, args...)...)
}
