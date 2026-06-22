//go:build debug

package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func initProfiles() func() {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8080", nil)

	runtime.SetMutexProfileFraction(10)

	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)

	cpuFile, err := os.Create("cpu." + timestamp + ".pprof")
	if err != nil {
		fmt.Printf("Could not create CPU profile: %v\n", err)
		return func() {}
	}
	pprof.StartCPUProfile(cpuFile)

	return func() {
		pprof.StopCPUProfile()
		cpuFile.Close()

		memFile, err := os.Create("mem." + timestamp + ".pprof")
		if err == nil {
			fmt.Printf("writing mem profile: %s\n", memFile.Name())
			runtime.GC()
			if err := pprof.WriteHeapProfile(memFile); err != nil {
				fmt.Printf("Could not write heap profile: %v\n", err)
			}
			memFile.Close()
		} else {
			fmt.Printf("Could not write heap profile: %v\n", err)
		}

		mutexFile, err := os.Create("mutex." + timestamp + ".pprof")
		if err == nil {
			fmt.Printf("writing mutex profile: %s\n", mutexFile.Name())
			if p := pprof.Lookup("mutex"); p != nil {
				if err := p.WriteTo(mutexFile, 0); err != nil {
					fmt.Printf("Could not write mutex profile: %v\n", err)
				}
			}
			mutexFile.Close()
		} else {
			fmt.Printf("Could not write mutex profile: %v\n", err)
		}
	}
}
