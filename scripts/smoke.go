package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	w := exec.Command("/tmp/spool-worker")
	w.Env = append(os.Environ(), "REDIS_ADDR="+addr)
	w.Stdout = os.Stdout
	w.Stderr = os.Stderr
	_ = w.Start()
	defer func() {
		if w.Process != nil {
			_ = w.Process.Kill()
			_ = w.Wait()
		}
	}()
	time.Sleep(2 * time.Second)
	out, _ := exec.Command("/tmp/spool", "run", "--image", "alpine", "echo", "hello").CombinedOutput()
	fmt.Print(string(out))
	id := ""
	for _, f := range strings.Fields(string(out)) {
		if strings.Count(f, "-") == 4 && len(f) == 36 {
			id = f
		}
	}
	if id == "" {
		os.Exit(1)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(time.Second)
		o, _ := exec.Command("/tmp/spool", "status", id).CombinedOutput()
		s := string(o)
		fmt.Print(s)
		if strings.Contains(s, "completed") && strings.Contains(s, "hello") {
			os.Exit(0)
		}
		if strings.Contains(s, "failed") {
			os.Exit(1)
		}
	}
	os.Exit(1)
}
