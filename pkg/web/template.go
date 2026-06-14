package web

import (
	"embed"
)

//go:embed templates/*
var templateFS embed.FS

type Site struct {
	Name     string
	URL      string
	Homepage string
	Issues   string
	Request  string
	Email    string
	Group    string
	Disk     string
	Note     string
	Big      string
}
