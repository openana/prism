//go:build !debug

package main

func initCPUProfile() func() { return func() {} }

func initMemProfile() func() { return func() {} }
