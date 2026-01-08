package filtering

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"

	"github.com/miekg/dns"
)

type parseOptions struct {
	ListID     string
	Logger     *slog.Logger
	ErrorLimit int
}

type errorLimiter struct {
	limit int
	count int
}

func parseList(r io.Reader, opts parseOptions) (*DomainSet, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	stats := ParseStats{}
	limiter := errorLimiter{limit: opts.ErrorLimit}
	set := NewDomainSet()

	scanner := bufio.NewScanner(r)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := stripBOM(scanner.Text())
		stats.TotalLines++
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isCommentLine(line) {
			continue
		}

		fields := splitFields(line)
		if len(fields) == 0 {
			continue
		}

		tokens := fields
		if ip := net.ParseIP(fields[0]); ip != nil {
			tokens = fields[1:]
		}

		addTokens(tokens, set, &stats, &limiter, logger, opts.ListID, lineNum)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan list: %w", err)
	}

	limiter.summary(logger, opts.ListID, stats.Invalid)
	logger.Info("parsed blocklist", "list", opts.ListID, "domains", stats.Domains, "invalid", stats.Invalid)
	return set, nil
}

func (l *errorLimiter) log(logger *slog.Logger, listID string, lineNum int, token string, err error) {
	if l.limit == 0 {
		return
	}
	if l.limit > 0 && l.count >= l.limit {
		l.count++
		return
	}
	l.count++
	logger.Error("invalid blocklist entry", "list", listID, "line", lineNum, "entry", token, "error", err)
}

func (l *errorLimiter) summary(logger *slog.Logger, listID string, invalid int) {
	if l.limit <= 0 {
		return
	}
	if invalid > l.limit {
		logger.Warn("blocklist parsing errors suppressed", "list", listID, "errors", invalid, "logged", l.limit)
	}
}

func stripBOM(line string) string {
	return strings.TrimPrefix(line, "\ufeff")
}

func splitFields(line string) []string {
	return strings.Fields(line)
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";")
}

func isCommentToken(token string) bool {
	return strings.HasPrefix(token, "#") || strings.HasPrefix(token, "//") || strings.HasPrefix(token, ";")
}

func addToken(set *DomainSet, token string) error {
	name := strings.TrimSpace(token)
	if name == "" {
		return fmt.Errorf("empty entry")
	}
	if strings.Contains(name, "://") || strings.Contains(name, "/") || strings.Contains(name, ":") {
		return fmt.Errorf("invalid hostname")
	}
	if ip := net.ParseIP(name); ip != nil {
		return fmt.Errorf("ip literals are not domains")
	}

	if strings.HasPrefix(name, "*.") {
		suffix := strings.TrimPrefix(name, "*.")
		canonical, err := normalizeDomain(suffix)
		if err != nil {
			return err
		}
		set.AddWildcard(canonical)
		return nil
	}

	canonical, err := normalizeDomain(name)
	if err != nil {
		return err
	}
	set.AddExact(canonical)
	return nil
}

func normalizeDomain(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("empty domain")
	}
	lower := strings.ToLower(strings.TrimSuffix(trimmed, "."))
	if lower == "" {
		return "", fmt.Errorf("empty domain")
	}
	if _, ok := dns.IsDomainName(lower); !ok {
		return "", fmt.Errorf("invalid domain")
	}
	return lower, nil
}

func addTokens(tokens []string, set *DomainSet, stats *ParseStats, limiter *errorLimiter, logger *slog.Logger, listID string, lineNum int) {
	for _, token := range tokens {
		if isCommentToken(token) {
			break
		}
		if err := addToken(set, token); err != nil {
			stats.Invalid++
			limiter.log(logger, listID, lineNum, token, err)
			continue
		}
		stats.Domains++
	}
}
