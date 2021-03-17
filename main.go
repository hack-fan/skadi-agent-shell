package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hack-fan/skadigo"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
)

// global
var log *zap.SugaredLogger
var settings = new(Settings)

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
	_ = configor.Load(settings, "skadi.yml", "/etc/skadi/skadi.yml")
	if settings.Token == "" {
		panic("token is required")
	}

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

	// context
	ctx, cancel := context.WithCancel(context.Background())

	// system signals - for graceful shutdown or restart
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Infof("%s signal received, send cancel to worker context.", sig)
		cancel()
	}()

	// skadi agent
	agent := skadigo.New(settings.Token, settings.Server, &skadigo.Options{
		Logger: log,
	})
	log.Info("Skadi agent start")
	agent.StartWorker(ctx, handler, 0)
	log.Info("this worker have been safety stopped.")
}
