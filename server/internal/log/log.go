package log

import (
	"log"
	"os"
)

// logger — глобальный логгер, пишущий в stdout с датой/временем (log.LstdFlags).
// Формат: "2006/01/02 15:04:05 сообщение".
// Используется всеми пакетами через обёртки F, Infof, Errorf.
var logger = log.New(os.Stdout, "", log.LstdFlags)

// F — kv-стиль логирования для структурированных сообщений.
// Параметры: msg — основное сообщение, kv — пары ключ-значение.
//
// Пример:
//
//	F("http server", "addr", ":8080", "env", "prod")
//	// Вывод: 2026/01/01 12:00:00 http server addr :8080 env prod
func F(msg string, kv ...any) {
	// Prepend msg к kv для единообразного вывода.
	args := append([]any{msg}, kv...)
	logger.Println(args...)
}

// Infof — информационное сообщение с форматированием (аналог log.Printf).
// Используется для штатных событий: запуск сервера, обработка запросов.
func Infof(format string, args ...any) { logger.Printf(format, args...) }

// Errorf — сообщение об ошибке с префиксом "ERROR:".
// Автоматически добавляет "ERROR: " к форматированной строке для удобства фильтрации в логах.
func Errorf(format string, args ...any) { logger.Printf("ERROR: "+format, args...) }
