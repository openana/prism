//go:build debug

package main

import (
	"os"
	"runtime/pprof"
	"strconv"
	"time"
)

// initCPUProfile starts CPU profiling and returns a stop function.
// The returned function stops profiling and closes the output file.
func initCPUProfile() func() {
	t := strconv.FormatInt(time.Now().UnixNano(), 10)
	f, err := os.Create(t + "-cpu.pprof")
	if err != nil {
		return func() {}
	}
	pprof.StartCPUProfile(f)
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}

// initMemProfile returns a function that writes the current heap profile
// to a timestamped file when called.
func initMemProfile() func() {
	return func() {
		t := strconv.FormatInt(time.Now().UnixNano(), 10)
		f, err := os.Create(t + "-mem.pprof")
		if err != nil {
			return
		}
		defer f.Close()
		pprof.WriteHeapProfile(f)
	}
}
