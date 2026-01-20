package ui

import "embed"

//go:embed templates/*
var TmplData embed.FS

//go:embed static/*
var StaticFS embed.FS
