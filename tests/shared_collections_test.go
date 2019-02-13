package tests

import (
	"encoding/json"
	"fmt"
	as "github.com/go-ap/activitystreams"
	"net/http"
	"os"
	"testing"
)

func Test_GETOutbox(t *testing.T) {
	apiURL := os.Getenv("API_URL")

	testId := fmt.Sprintf("%s/self/outbox", apiURL)
	testFirstId := fmt.Sprintf("%s/self/outbox?maxItems=50&page=1", apiURL)

	assertTrue := errIfNotTrue(t)

	url := fmt.Sprintf("%s/self/outbox", apiURL)
	var b []byte
	var err error
	b, err = execReq(url, http.MethodGet, nil)
	assertTrue(err == nil, "Error %s", err)
	test := make(map[string]interface{})
	err = json.Unmarshal(b, &test)
	assertTrue(err == nil, "Error unmarshal: %s", err)

	for key, val := range test {
		if key == "id" {
			assertTrue(val == testId, "Invalid id, %s expected %s", val, testId)
		}
		if key == "type" {
			assertTrue(val == string(as.OrderedCollectionType), "Invalid type, %s expected %s", val, as.OrderedCollectionType)
		}
		if key == "totalItems" {
			v, ok := val.(float64)
			assertTrue(ok, "Unable to convert to %T type. Expected float value %v:%T", v, val, val)
			assertTrue(int64(v) == 1, "Invalid totalItems, %d expected %d", int64(v), 1)
		}
		if key == "first" {
			assertTrue(val == testFirstId, "Invalid first collection page id, %s expected %s", val, testFirstId)
		}
	}
}

func Test_GETInbox(t *testing.T) {
	apiURL := os.Getenv("API_URL")

	testId := fmt.Sprintf("%s/self/inbox", apiURL)
	testFirstId := fmt.Sprintf("%s/self/inbox?maxItems=50&page=1", apiURL)

	assertTrue := errIfNotTrue(t)

	url := fmt.Sprintf("%s/self/inbox", apiURL)
	var b []byte
	var err error
	b, err = execReq(url, http.MethodGet, nil)
	assertTrue(err == nil, "Error %s", err)

	test := make(map[string]interface{})
	err = json.Unmarshal(b, &test)
	assertTrue(err == nil, "Error unmarshal: %s", err)

	for key, val := range test {
		if key == "id" {
			assertTrue(val == testId, "Invalid id, %s expected %s", val, testId)
		}
		if key == "type" {
			assertTrue(val == string(as.OrderedCollectionType), "Invalid type, %s expected %s", val, as.OrderedCollectionType)
		}
		if key == "totalItems" {
			v, ok := val.(float64)
			assertTrue(ok, "Unable to convert to %T type. Expected float value %v:%T", v, val, val)
			assertTrue(int64(v) == 1, "Invalid totalItems, %d expected %d", int64(v), 1)
		}
		if key == "first" {
			assertTrue(val == testFirstId, "Invalid first collection page id, %s expected %s", val, testFirstId)
		}
	}
}
