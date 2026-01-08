package filtering

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type blockedLogger struct {
	file *os.File
	mu   sync.Mutex
}

func newBlockedLogger(path string, log *slog.Logger) *blockedLogger {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- path provided via config.
	if err != nil {
		if log == nil {
			slog.Default().Error("failed to open blocked log file", "error", err)
		} else {
			log.Error("failed to open blocked log file", "error", err)
		}
		return nil
	}
	return &blockedLogger{file: file}
}

func (b *blockedLogger) Log(remoteAddr string, name string, qtype uint16) {
	if b == nil || b.file == nil {
		return
	}
	recordType := dns.TypeToString[qtype]
	if recordType == "" {
		recordType = fmt.Sprintf("%d", qtype)
	}
	line := fmt.Sprintf("%s client=%s type=%s name=%s\n",
		time.Now().UTC().Format(time.RFC3339),
		remoteAddr,
		recordType,
		name,
	)
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.file.WriteString(line)
}
