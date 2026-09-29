package main

import (
	"log"

	"polymarket-collector/collector"
)

func main() {
	if err := collector.Run(); err != nil {
		log.Fatal(err)
	}
}
