package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nak")
	build := exec.Command("go", "build", "-o", bin, "..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	return bin
}

// TestBinaryServesMCPOverStdio builds the real binary and talks JSON-RPC to it
// the way an MCP client does — no credentials needed for initialize/tools/list.
func TestBinaryServesMCPOverStdio(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "mcp")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "CIS_USER=", "CIS_PASS=", "MOODLE_USER=", "MOODLE_PASS=", "NAK_READONLY=1")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	r := bufio.NewReader(out)
	send := func(m map[string]any) {
		b, _ := json.Marshal(m)
		in.Write(append(b, '\n'))
	}
	recv := func() map[string]any {
		done := make(chan map[string]any, 1)
		go func() {
			line, _ := r.ReadString('\n')
			var m map[string]any
			json.Unmarshal([]byte(line), &m)
			done <- m
		}()
		select {
		case m := <-done:
			return m
		case <-time.After(10 * time.Second):
			t.Fatal("no answer from MCP server")
			return nil
		}
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	init := recv()
	res, _ := init["result"].(map[string]any)
	if res == nil || !strings.Contains(res["instructions"].(string), "confirm=true") {
		t.Fatalf("initialize = %v", init)
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	list := recv()
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 75 {
		t.Fatalf("tools/list returned %d tools", len(tools))
	}
	// A tool needing credentials must fail gracefully, not crash the server.
	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "moodle_whoami", "arguments": map[string]any{}}})
	call := recv()
	cr, _ := call["result"].(map[string]any)
	if cr == nil || cr["isError"] != true {
		t.Fatalf("call without credentials = %v", call)
	}
}
