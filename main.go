package main

import (
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
)

const defaultVersion = "2.0"

var (
	s               *discordgo.Session
	commandHandlers map[string]func(*discordgo.InteractionCreate, discordgo.ApplicationCommandInteractionData)
)

func init() {
	// testing only
	// t := time.Now().UTC().Add(-time.Hour)
	// os.WriteFile("time.txt", []byte(t.Format(time.RFC3339Nano)), 0644)

	file, err := os.ReadFile("token.txt")
	if err != nil {
		panic(err)
	}
	token := string(file)
	s, err = discordgo.New("Bot " + token)
	if err != nil {
		panic(err)
	}
}

func main() {
	log.Println("Initializing Commands")
	commands, handlers := InitCommands()
	commandHandlers = handlers

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) { log.Println("READY") })
	s.AddHandler(GuildCreate)
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		data := i.ApplicationCommandData()
		commandHandlers[data.Name](i, data)
	})

	if err := s.Open(); err != nil {
		panic(err)
	}
	defer s.Close()

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	if err != nil {
		panic(err)
	}

	log.Println("Initializing Updates")
	go func() {
		for {
			UpdateMods()
			time.Sleep(time.Minute)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("Shutting down...")
}
