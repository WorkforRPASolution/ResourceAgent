// fake_daemon.go is a test helper that simulates LhmHelper.exe --daemon protocol.
// It is compiled and run by daemon tests.
//
// Behavior is controlled by the FAKE_DAEMON_MODE env var:
//
//	"normal"    - Respond to each stdin line with valid JSON (default)
//	"crash"     - Respond once then exit
//	"slow"      - Read stdin but never respond (for timeout testing)
//	"error"     - Respond with error JSON
//	"open_fail" - Write error JSON immediately and exit (simulates computer.Open failure)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type lhmData struct {
	Sensors          []sensor     `json:"Sensors"`
	Fans             []fan        `json:"Fans"`
	Gpus             []gpu        `json:"Gpus"`
	Storages         []storage    `json:"Storages"`
	Voltages         []voltage    `json:"Voltages"`
	MotherboardTemps []mbTemp     `json:"MotherboardTemps"`
	Error            string       `json:"error,omitempty"`
}

type sensor struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
	High        float64 `json:"High"`
	Critical    float64 `json:"Critical"`
}

type fan struct {
	Name string  `json:"Name"`
	RPM  float64 `json:"RPM"`
}

type gpu struct {
	Name        string   `json:"Name"`
	Temperature *float64 `json:"Temperature"`
}

type storage struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
}

type voltage struct {
	Name    string  `json:"Name"`
	Voltage float64 `json:"Voltage"`
}

type mbTemp struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
}

func main() {
	mode := os.Getenv("FAKE_DAEMON_MODE")
	if mode == "" {
		mode = "normal"
	}

	switch mode {
	case "open_fail":
		b, _ := json.Marshal(lhmData{Error: "computer.Open() failed: PawnIO driver not found"})
		fmt.Println(string(b))
		os.Exit(1)
	case "slow":
		// Read stdin but never respond
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			// do nothing
		}
		os.Exit(0)
	}

	temp := 65.0
	sampleData := lhmData{
		Sensors: []sensor{
			{Name: "CPU Package", Temperature: temp, High: 100, Critical: 105},
		},
		Fans:     []fan{{Name: "CPU Fan", RPM: 1200}},
		Gpus:     []gpu{{Name: "Intel HD Graphics", Temperature: &temp}},
		Storages: []storage{{Name: "Samsung SSD", Type: "SSD"}},
		Voltages: []voltage{{Name: "CPU Vcore", Voltage: 1.25}},
		MotherboardTemps: []mbTemp{{Name: "System", Temperature: 42}},
	}

	errorData := lhmData{Error: "sensor read failed"}

	scanner := bufio.NewScanner(os.Stdin)
	requestCount := 0

	for scanner.Scan() {
		requestCount++

		switch mode {
		case "normal":
			b, _ := json.Marshal(sampleData)
			fmt.Println(string(b))
		case "crash":
			if requestCount == 1 {
				b, _ := json.Marshal(sampleData)
				fmt.Println(string(b))
			}
			// Exit after first response (simulates crash)
			if requestCount >= 2 {
				os.Exit(1)
			}
		case "error":
			b, _ := json.Marshal(errorData)
			fmt.Println(string(b))
		}
	}
}
