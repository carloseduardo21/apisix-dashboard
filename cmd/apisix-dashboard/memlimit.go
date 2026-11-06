package main

import (
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// Files holding the container memory limit (cgroup v2, then v1).
var cgroupMemoryFiles = []string{
	"/sys/fs/cgroup/memory.max",
	"/sys/fs/cgroup/memory/memory.limit_in_bytes",
}

// memLimitShare is the part of the container limit given to the Go heap; the
// rest covers stacks, runtime overhead and memory not yet returned to the OS.
const memLimitShare = 0.85

// setMemoryLimit sets a soft Go memory limit from the container limit unless
// GOMEMLIMIT is set. Each argon2id hash allocates 64 MiB; without a soft
// limit the garbage of finished hashes accumulates (GOGC=100) and the
// container is OOM-killed under a burst of logins.
func setMemoryLimit() {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	for _, f := range cgroupMemoryFiles {
		b, err := os.ReadFile(f) // #nosec G304 -- fixed cgroup paths
		if err != nil {
			continue
		}
		if limit, ok := parseCgroupLimit(string(b)); ok {
			soft := int64(float64(limit) * memLimitShare)
			debug.SetMemoryLimit(soft)
			slog.Info("memory limit set from the container limit", "container_bytes", limit, "go_soft_limit_bytes", soft)
		}
		return
	}
}

// parseCgroupLimit returns the limit in bytes, or false when there is none
// ("max" in v2, a huge page-aligned value in v1).
func parseCgroupLimit(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n >= 1<<60 {
		return 0, false
	}
	return n, true
}
