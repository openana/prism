//go:build !debug

package main

func initProfiles() func() { return func() {} }
