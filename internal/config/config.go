package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// Table maps normalized suffix (lowercase, no trailing dot) to IPv4.
type Table struct {
	entries map[string]net.IP
}

// Load reads path line by line: domain;IPv4. Lines starting with # after trim are comments.
// Empty lines are skipped. Invalid lines are skipped; warn is called for each skip if non-nil.
func Load(path string, warn func(line int, msg string)) (*Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	t := &Table{entries: make(map[string]net.IP)}
	sc := bufio.NewScanner(f)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := strings.TrimSuffix(sc.Text(), "\r")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ";", 2)
		if len(parts) != 2 {
			if warn != nil {
				warn(lineNum, "expected domain;ip")
			}
			continue
		}
		suffix := NormalizeSuffix(parts[0])
		if suffix == "" {
			if warn != nil {
				warn(lineNum, "empty domain")
			}
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(parts[1]))
		if ip == nil || ip.To4() == nil {
			if warn != nil {
				warn(lineNum, "invalid IPv4")
			}
			continue
		}
		t.entries[suffix] = ip.To4()
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return t, nil
}

// NormalizeSuffix lowercases and removes trailing root dot.
func NormalizeSuffix(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	return strings.ToLower(s)
}

// NormalizeQuery normalizes the QNAME from a DNS question.
func NormalizeQuery(fqdn string) string {
	return NormalizeSuffix(fqdn)
}

// Lookup returns the IPv4 for the longest suffix match against q (FQDN from question).
func (t *Table) Lookup(q string) (net.IP, bool) {
	if t == nil {
		return nil, false
	}
	qn := NormalizeQuery(q)
	for cur := qn; cur != ""; {
		if ip, ok := t.entries[cur]; ok {
			return ip, true
		}
		i := strings.IndexByte(cur, '.')
		if i < 0 {
			break
		}
		cur = cur[i+1:]
	}
	return nil, false
}

// Len returns the number of suffix entries loaded.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.entries)
}
