package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type MacLocation struct {
	County    string `json:"county"`
	Locality  string `json:"locality"`
	Longitude string `json:"longitude"`
	Street    string `json:"street"`
	Region    string `json:"region"`
	Postcode  string `json:"postcode"`
	Latitude  string `json:"latitude"`
	Altitude  string `json:"altitude"`
}

// GetLocationFromShortcut gets the location of the device from the Shortcuts app
// This seems to be fairly unreliable, unfortunately.
func GetLocationFromShortcut(output chan<- string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/shortcuts", "run", "getCoreLocationData").Output()
	if err != nil {
		output <- fmt.Sprintf("Error: %s", err)
	}
	output <- string(out)
}

func ParseLocation(location string) string {
	loc := MacLocation{}
	err := json.Unmarshal([]byte(location), &loc)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s, %s", loc.Locality, loc.County)
}
