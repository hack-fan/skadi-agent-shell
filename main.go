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
	"time"

	"github.com/hack-fan/skadigo"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// global
var log *zap.SugaredLogger
var settings = new(Settings)
var etcFiles = []string{"skadi.yml", "/etc/skadi/skadi.yml"}

// Settings will be load from /etc/skadi/skadi.yml
type Settings struct {
	Debug  bool `default:"false"`
	Token  string
	Server string `default:"https://api.letserver.run"`
	// Shortcuts convert a single word to shell command
	// Short: input message, required
	// Dir: working directory, optional
	// Cmd: whole command line, required
	Shortcuts []struct {
		Short string
		Dir   string
		Cmd   string
	}
	// Commands is a command white list
	// Dir: working directory, optional
	// Prefix: if the input message has the prefix, run it
	Commands []struct {
		Dir    string
		Prefix string
	}
	// Templates is a printf style template, this is unsafe
	// Name: if the first word of input message hit name, run it
	// Dir: working directory, optional
	// Temp: render the args to this template string
	// Example | Name:"Hi" Temp:"echo I am not %s!"
	// Message [Hi Jim] Result [I am not Jim!]
	Templates []struct {
		Name string
		Dir  string
		Temp string
	}
}

func (s *Settings) CommandsText() string {
	var res string
	if s.Shortcuts != nil && len(s.Shortcuts) > 0 {
		res += "\n\nShortcuts:"
		for _, item := range s.Shortcuts {
			res += fmt.Sprintf("\n - %s: %s ", item.Short, item.Cmd)
			if len(item.Dir) > 0 {
				res += fmt.Sprintf("[%s]", item.Dir)
			}
		}
	}
	if s.Commands != nil && len(s.Commands) > 0 {
		res += "\n\nCommands:"
		for _, cmd := range s.Commands {
			res += fmt.Sprintf("\n - %s ", cmd.Prefix)
			if len(cmd.Dir) > 0 {
				res += fmt.Sprintf("[%s]", cmd.Dir)
			}
		}
	}
	if s.Templates != nil && len(s.Templates) > 0 {
		res += "\n\nTemplates:"
		for _, item := range s.Templates {
			res += fmt.Sprintf("\n - %s: %s ", item.Name, item.Temp)
			if len(item.Dir) > 0 {
				res += fmt.Sprintf("[%s]", item.Dir)
			}
		}
	}
	return res
}

func handler(id, msg string) (string, error) {
	log.Debugf("received command message[%s]: %s", id, msg)
	// default error
	e := fmt.Errorf("unsupported command: %s", msg)
	// parse msg
	if msg == "help" || msg == "all" {
		return settings.CommandsText(), nil
	}
	// shortcut
	for _, item := range settings.Shortcuts {
		if msg == item.Short {
			return run(item.Cmd, item.Dir)
		}
	}
	// command
	for _, item := range settings.Commands {
		if strings.HasPrefix(msg, item.Prefix) {
			return run(msg, item.Dir)
		}
	}
	// template
	a := strings.Split(msg, " ")
	if len(a) > 1 {
		for _, item := range settings.Templates {
			if a[0] == item.Name {
				b := make([]interface{}, len(a)-1)
				for i, v := range a[1:] {
					b[i] = v
				}
				return run(fmt.Sprintf(item.Temp, b...), item.Dir)
			}
		}
	}
	// other
	log.Error(e)
	return "", e
}

func run(cmd, dir string) (string, error) {
	var command = exec.Command("sh", "-c", cmd)
	if dir != "" {
		command.Dir = dir
	}
	log.Debugf("command: %+v", command)
	res, err := command.CombinedOutput()
	if err != nil {
		e := fmt.Errorf("run command failed, %w : %s", err, res)
		log.Error(e)
		return string(res), e
	}
	log.Infof("%s", res)
	return string(res), nil
}

func main() {
	// load config, user config in workdir has high priority
	_ = configor.New(&configor.Config{Silent: true}).Load(settings, etcFiles...)

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
	// only change the first found etc file
	if len(os.Args) == 2 {
		token := os.Args[1]
		if len(token) != 20 {
			log.Fatalf("invalid token: %s", token)
		}
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
			os.Exit(0)
		}
		log.Fatal("skadi.yml not found")
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
	err = agent.Start(ctx, handler, time.Second*10)
	if err != nil {
		log.Errorf("skadi shell agent will exit with error: %s", err)
		os.Exit(1)
	}

	// context dead
	log.Info("this worker have been safety stopped.")
}
