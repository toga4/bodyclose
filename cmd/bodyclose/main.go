package main

import (
	"github.com/toga4/bodyclose"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() {
	unitchecker.Main(bodyclose.Analyzer)
}
