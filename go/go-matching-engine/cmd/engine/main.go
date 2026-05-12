package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kenlau/go-matching-engine/engine"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "engine:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	eng := engine.New()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	bw := bufio.NewWriter(out)
	defer bw.Flush()
	enc := json.NewEncoder(bw)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev engine.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		trades, err := eng.Step(ev)
		if err != nil {
			return fmt.Errorf("step: %w", err)
		}
		for i := range trades {
			if err := enc.Encode(&trades[i]); err != nil {
				return fmt.Errorf("encode trade: %w", err)
			}
		}
	}
	return scanner.Err()
}
