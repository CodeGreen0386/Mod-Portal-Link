package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
)

type info_json struct {
	Factorio_version string
}

type latest_release struct {
	Download_url string
	File_name 	 string
	Info_json 	 info_json
	Released_at  string
	Version 	 string
	Sha1 		 string
}

type mod struct {
	Name 			string
	Title 			string
	Owner 			string
	Summary 		string
	Downloads_count int
	Category 		string
	Score 			float32
	Latest_release  latest_release
}

type response struct {
	Pagination map[string]string `json:"pagination"`
	Results    []mod             `json:"results"`
}

var (
	s    *discordgo.Session
	mods map[string]map[string]mod
	versions []string
)

func init() {
	file, err := os.ReadFile("token.txt")
	if err != nil {log.Fatal(err)}
	token := string(file)
	s, err = discordgo.New("Bot " + token)
	if err != nil {log.Fatalf("Invalid token: %v", token)}

	mods = make(map[string]map[string]mod)
	versions := [...]string{"1.1", "1.0", "0.18", "0.17", "0.16", "0.15", "0.14", "0.13"}
	for _, version := range(versions) {
		mods[version] = make(map[string]mod)
	}
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name: "mod",
			Description: "Links a mod",
			Type: discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name: "name",
					Description: "Mod name",
					Type: discordgo.ApplicationCommandOptionString,
					Required: true,
					Autocomplete: true,
				},
				{
					Name: "author",
					Description: "Author",
					Type: discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"mod": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			switch i.Type {
			case discordgo.InteractionApplicationCommand:
				data := i.ApplicationCommandData()
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "https://mods.factorio.com/mod/" + data.Options[0].StringValue(),
					},
				})
				if err != nil {
					panic(err)
				}
			case discordgo.InteractionApplicationCommandAutocomplete:
				// data := i.ApplicationCommandData()
				choices := []*discordgo.ApplicationCommandOptionChoice{
					{
						Name: "Choice One",
						Value: "one",
					},
					{
						Name: "Choice Two",
						Value: "two",
					},
					{
						Name: "Choice Three",
						Value: "three",
					},
				}

				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionApplicationCommandAutocompleteResult,
					Data: &discordgo.InteractionResponseData{
						Choices: choices,
					},
				})
				if err != nil {
					panic(err)
				}
			}
		},
	}
)

type ModSort []mod
func (m ModSort) Swap(a, b int) {
	m[a], m[b] = m[b], m[a]
}
func (m ModSort) Less(a, b int) bool {
	a_internal := m[a].Category == "internal"
	b_internal := m[b].Category == "internal"
	if a_internal != b_internal {
		return b_internal
	}
	return m[a].Downloads_count > m[b].Downloads_count
}

func CompareCache(data []mod) {
	// updated := make(map[string]mod)
	file, err := os.ReadFile("mods.json")
	if err != nil {panic(err)}
	var cache []mod
	if err := json.Unmarshal(file, &cache); err != nil {panic(err)}

	for i := 0; i < len(data); i++ {
		log.Println(data[i].Name)
	}
}

func UpdateCache() {
	resp, err := http.Get("https://mods.factorio.com/api/mods?page_size=max")
	if err != nil {panic(err)}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {panic(err)}

	var data response
	if err := json.Unmarshal(body, &data); err != nil {panic(err)}
	CompareCache(data.Results)

	file, err := json.MarshalIndent(data.Results, "", "    ")
	if err != nil {panic(err)}
	os.WriteFile("mods.json", file, 0644)

	log.Println("Updated mods.json")
}

func main() {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {log.Println("READY")})
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	if err := s.Open(); err != nil {
		log.Fatalf("Cannot open session: %v", err)
	}
	defer s.Close()

	createdCommands, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	if err != nil {
		log.Fatalf("Cannot register commands: %v", err)
	}
	go func() {
		for {
			UpdateCache()
			time.Sleep(time.Minute * 5)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	for _, cmd := range createdCommands {
		if err := s.ApplicationCommandDelete(s.State.User.ID, "", cmd.ID); err != nil {
			log.Fatalf("Cannot delete %q command: %v", cmd.Name, err)
		}
	}

	log.Println("Shutting down")
}