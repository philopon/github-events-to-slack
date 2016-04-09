package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/philopon/github-events-to-slack/github"
	"github.com/philopon/github-events-to-slack/slack"

	"gopkg.in/alecthomas/kingpin.v2"
)

type Config struct {
	Slack struct {
		Token   string
		Channel string
	}
	Github struct {
		Token string
		User  string
	}
}

func Watch(config Config, state string) error {
	gh := github.NewGithub(config.Github.Token, config.Github.User)
	err := gh.LoadState(state)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WARNING: state file parseing failed")
	}

	interupt := make(chan os.Signal, 1)
	signal.Notify(interupt, os.Interrupt)
	go func() {
		<-interupt
		gh.SaveState(state)
		os.Exit(1)
	}()

	sl := slack.NewSlack(config.Slack.Token, config.Slack.Channel)

	go gh.Polling()

	go func() {
		for err := range gh.Errors {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	for event := range gh.Events {
		msg, err := slack.ParseEvent(event)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			if event.Public {
				err := sl.UploadEvent(event, err.Error())
				if err != nil {
					gh.Errors <- err
				}
			}
			continue
		}

		sl.PostMessage(msg)
	}

	return nil
}

func Single(config Config, file io.Reader) error {
	var event github.Event
	err := json.NewDecoder(file).Decode(&event)
	if err != nil {
		return err
	}

	msg, err := slack.ParseEvent(event)
	if err != nil {
		return err
	}

	sl := slack.NewSlack(config.Slack.Token, config.Slack.Channel)
	sl.PostMessage(msg)

	return nil
}

func main() {
	var err error

	app := kingpin.New("github-events-to-slack", "")
	configFile := app.Flag("config", "config file").Short('c').Default("config.json").File()

	watch := app.Command("watch", "watch")
	state := watch.Flag("state", "state file").Short('s').Default(".state").String()

	single := app.Command("single", "post from single event json file")
	singleJson := single.Arg("JSON", "event json").Required().File()

	mode := kingpin.MustParse(app.Parse(os.Args[1:]))

	var config Config
	err = json.NewDecoder(*configFile).Decode(&config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch mode {
	case watch.FullCommand():
		err = Watch(config, *state)
	case single.FullCommand():
		err = Single(config, *singleJson)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
