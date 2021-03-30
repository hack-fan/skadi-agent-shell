package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hack-fan/skadigo"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// global
var log *zap.SugaredLogger
var settings = new(Settings)
var etcFiles = []string{"skadi.yml", "/etc/skadi/skadi.yml"}

// Setting will be load from /etc/skadi/skadi.yml
type Settings struct {
	Debug    bool `default:"false"`
	Token    string
	Server   string `default:"https://api.letserver.run"`
	Commands []struct {
		Msg string
		Dir string
		Cmd string
	}
	Templates []struct {
		Name string
		Dir  string
		Temp string
	}
}

func (s *Settings) CommandsText() string {
	if s.Commands == nil || len(s.Commands) == 0 {
		return "No commands defined."
	}
	res := "All commands:\n"
	for _, cmd := range s.Commands {
		res += fmt.Sprintf("\n[%s] %s (%s)", cmd.Msg, cmd.Cmd, cmd.Dir)
	}
	return res
}

func handler(msg string) (string, error) {
	log.Debugf("received command message: %s", msg)
	// default error
	e := fmt.Errorf("unsupported command: %s", msg)
	// parse command
	if msg == "help" || msg == "all" {
		return settings.CommandsText(), nil
	}
	for _, cmd := range settings.Commands {
		if msg == cmd.Msg {
			return run(cmd.Cmd, cmd.Dir)
		}
	}
	// parse template
	a := strings.Split(msg, " ")
	if len(a) > 1 {
		for _, temp := range settings.Templates {
			if a[0] == temp.Name {
				return run(fmt.Sprintf(temp.Temp, strings.Join(a[1:], " ")), temp.Dir)
			}
		}
	}
	// other
	log.Error(e)
	return "", e
}

func run(cmd, dir string) (string, error) {
	ca := strings.Split(cmd, " ")
	var command = exec.Command(ca[0], ca[1:]...)
	if dir != "" {
		command.Dir = dir
	}
	log.Debugf("command: %+v", command)
	res, err := command.Output()
	if err != nil {
		log.Error(err)
		return "", err
	}
	log.Infof("%s", res)
	return string(res), nil
}

func main() {
	// load config, user config in workdir has high priority
	_ = configor.Load(settings, etcFiles...)

	// logger
	var logger *zap.Logger
	var err error
	if settings.Debug {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		panic(err)
	}
	defer logger.Sync() // nolint
	log = logger.Sugar()

	// if there is one arg, set it as token, then exit.
	if len(os.Args) == 2 {
		token := os.Args[1]
		if len(token) != 20 {
			log.Fatalf("invalid token: %s", token)
		}
		var changed bool
		for _, fname := range etcFiles {
			f, err := ioutil.ReadFile(fname)
			if err != nil {
				continue
			}
			var node yaml.Node
			err = yaml.Unmarshal(f, &node)
			if err != nil {
				log.Fatalf("parse etc file [%s] failed: %s", fname, err)
			}
			tmp, _ := json.MarshalIndent(node, "", "  ")
			log.Debug(string(tmp))
			if node.Content == nil {
				log.Fatalf("bad etc file [%s]", fname)
			}
			var found bool
			for _, item := range node.Content[0].Content {
				// token key found in last loop
				if found {
					item.Style = yaml.DoubleQuotedStyle
					item.Tag = "!!str"
					item.Value = token
					break
				}
				// find the key, change value in next loop
				if item.Value == "token" {
					found = true
				}
			}
			if err != nil {
				log.Fatalf("add token to etc file [%s] failed: %s", fname, err)
			}
			nf, err := yaml.Marshal(&node)
			if err != nil {
				log.Fatalf("add token to etc file [%s] failed: %s", fname, err)
			}
			err = ioutil.WriteFile(fname, nf, 0644)
			if err != nil {
				log.Fatalf("write token to etc file [%s] failed: %s", fname, err)
			}

			changed = true
		}
		if !changed {
			log.Fatal("skadi.yml not found")
		}
		os.Exit(0)
	}

	// check token
	if settings.Token == "" {
		log.Fatal("token is required")
	}

	// system signals - for graceful shutdown or restart
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// skadi agent
	agent := skadigo.New(settings.Token, settings.Server, &skadigo.Options{
		Logger: log,
	})
	log.Info("Skadi agent start")
	// blocked
	agent.StartWorker(ctx, handler, 0)

	// context dead
	log.Info("this worker have been safety stopped.")
}
