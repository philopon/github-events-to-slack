package github

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

type state struct {
	Etag     *string
	Last     time.Time
	Interval time.Duration
}

type github struct {
	state
	user     string
	client   *http.Client
	endpoint string
	Events   chan Event
	Errors   chan error
}

func NewGithub(token string, user string) *github {
	state := state{Last: time.Unix(0, 0), Interval: 60 * time.Second}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	client := oauth2.NewClient(oauth2.NoContext, ts)

	endpoint := "https://api.github.com/users/" + user + "/received_events"

	return &github{state, user, client, endpoint, make(chan Event), make(chan error)}
}

func (gh *github) SaveState(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("github.SaveState: %v", err)
	}
	defer file.Close()

	gob.NewEncoder(file).Encode(&gh.state)

	return nil
}

func (gh *github) LoadState(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var state state
	err = gob.NewDecoder(file).Decode(&state)
	if err != nil {
		return fmt.Errorf("github.LoadState: %v", err)
	}

	gh.state = state

	return nil
}

type Parsed struct {
	Type  string
	Actor struct {
		Login     string
		AvatarUrl string `json:"avatar_url"`
	}
	Repo struct {
		Name string
	}
	Payload   json.RawMessage
	CreatedAt time.Time `json:"created_at"`
	Public    bool
}

type Event struct {
	Parsed
	Raw json.RawMessage
}

func (e Event) String() string {
	return fmt.Sprintf("{EventInfo:%+v, RawEvent:<<RAW>>}", e.Parsed)
}

func (e *Event) UnmarshalJSON(data []byte) error {
	var parsed Parsed
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		return err
	}

	e.Parsed = parsed
	e.Raw = data

	return nil
}

func (gh *github) createRequest() *http.Request {
	req, err := http.NewRequest("GET", gh.endpoint, nil)
	if err != nil {
		panic(fmt.Sprintf("BUG: github.createRequest: %v", err))
	}

	if gh.Etag != nil {
		req.Header.Add("If-None-Match", *gh.Etag)
	}

	return req
}

func (gh *github) updateState(hdr http.Header) error {
	etag, ok := hdr["Etag"]
	if ok && len(etag) > 0 {
		gh.Etag = &etag[0]
	}

	intervalString, ok := hdr["X-Poll-Interval"]
	if ok && len(intervalString) > 0 {
		interval, err := strconv.Atoi(intervalString[0])
		if err != nil {
			return err
		}
		gh.Interval = time.Duration(interval) * time.Second
	}

	return nil
}

func (gh *github) Query() ([]Event, error) {
	resp, err := gh.client.Do(gh.createRequest())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = gh.updateState(resp.Header)
	if err != nil {
		return nil, err
	}

	var allEvents []Event
	json.NewDecoder(resp.Body).Decode(&allEvents)

	events := make([]Event, 0, len(allEvents))

	for i := range allEvents {
		event := allEvents[len(allEvents)-1-i]

		if !event.CreatedAt.After(gh.Last) {
			continue
		}

		gh.Last = event.CreatedAt
		events = append(events, event)
	}

	return events, nil
}

func (gh *github) Polling() {
	for {
		events, err := gh.Query()
		if err != nil {
			gh.Errors <- err
		} else {
			for _, event := range events {
				gh.Events <- event
			}
		}

		time.Sleep(gh.Interval)
	}
}
