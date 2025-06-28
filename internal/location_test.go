package internal

import (
	"testing"
)

func TestParseLocation(t *testing.T) {
	payload := `{"county":"OR","locality":"Portland","longitude":"-122.676483","street":"1 Some Way","region":"United States","postcode":"97001","latitude":"45.523064","altitude":"0"}`
	if ParseLocation(payload) != "Portland, OR" {
		t.Errorf("ParseLocation returned invalid payload: %s", payload)
	}
}

//func TestGetLocationExample(t *testing.T) {
//	chanLocation := make(chan string, 1)
//	go GetLocation(chanLocation)
//  // Wait for location or timeout
//	x := <-chanLocation
//	fmt.Println(x)
//	fmt.Println(ParseLocation(x))
//}
