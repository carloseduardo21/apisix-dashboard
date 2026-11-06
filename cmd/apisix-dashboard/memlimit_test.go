package main

import "testing"

func TestParseCgroupLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"268435456\n", 268435456, true},
		{"max\n", 0, false},
		{"", 0, false},
		{"9223372036854771712", 0, false}, // cgroup v1 "unlimited"
		{"-1", 0, false},
		{"garbage", 0, false},
	}
	for _, tc := range tests {
		got, ok := parseCgroupLimit(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseCgroupLimit(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
