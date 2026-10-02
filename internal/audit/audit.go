// Package audit appends one line per state-changing request to
// <dir>/writes.log — target and field names only, never values.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex

func Log(dir, system, method, target string, fieldNames []string) {
	if dir == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, "writes.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s %s fields=%s\n", time.Now().Format(time.RFC3339), system, method, target, strings.Join(fieldNames, ","))
}
