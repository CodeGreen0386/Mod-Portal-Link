package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type response struct {
    Pagination map[string]string `json:"pagination"`
    Results    ModArr            `json:"results"`
}

type ModArr []Mod
type Mod struct {
    Name 			string
    Title 			string
    Owner 			string
    Summary 		string
    Downloads_count int
    Category 		string
    Score 			float32
    Latest_release  latest_release
}

type latest_release struct {
    Download_url string
    File_name 	 string
    Info_json 	 info_json
    Released_at  string
    Version 	 string
    Sha1 		 string
}

type info_json struct {
    Factorio_version string
}

var (
	s        *discordgo.Session
    mods     map[string]Mod
    authors  map[string]ModArr
	versions map[string]ModArr
)

var version_list = [...]string{"1.1", "1.0", "0.18", "0.17", "0.16", "0.15", "0.14", "0.13"}

func init() {
    file, err := os.ReadFile("token.txt")
    if err != nil {log.Fatal(err)}
    token := string(file)
    s, err = discordgo.New("Bot " + token)
    if err != nil {log.Fatalf("Invalid token: %v", token)}
	
	mods = make(map[string]Mod)
	authors = make(map[string]ModArr)
	versions = make(map[string]ModArr)
	for _, version := range(version_list) {
		versions[version] = make(ModArr, 0)
	}
}

func VersionFilter(version string) ModArr {
    if newList, ok := versions[version]; ok {
        return newList
    }
    return versions["any"]
}

func AuthorFilter(modList ModArr, author string) ModArr {
    if author == "" {return modList}
    author = strings.ToLower(author)
    if _, ok := authors[author]; !ok {
        return modList
    }
    newList := make(ModArr, 0)
    for _, mod := range modList {
        if strings.ToLower(mod.Owner) == author {
            newList = append(newList, mod)
        }
    }
    return newList
}

type ApplicationCommandInteractionData discordgo.ApplicationCommandInteractionData
func FocusedOption(data discordgo.ApplicationCommandInteractionData) (*discordgo.ApplicationCommandInteractionDataOption, error) {
    for _, option := range(data.Options) {
        if option.Focused {
            return option, nil
        }
    }
    return nil, errors.New("No focused option found")
}

func NamedOption(data discordgo.ApplicationCommandInteractionData, name string) (*discordgo.ApplicationCommandInteractionDataOption, error) {
	for _, option := range(data.Options) {
		if option.Name == name {
			return option, nil
		}
	}
	return nil, errors.New("No named option found")
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
                {
                    Name: "version",
                    Description: "Version",
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
                if err != nil {panic(err)}
            case discordgo.InteractionApplicationCommandAutocomplete:
                data := i.ApplicationCommandData()
                choices := []*discordgo.ApplicationCommandOptionChoice{}
                focusedOption, err := FocusedOption(data)
                if err != nil {panic(err)}

				log.Println(focusedOption.Name)
                switch focusedOption.Name {
                case "name":
					choices = []*discordgo.ApplicationCommandOptionChoice{
						{
							Name: "Mod 1",
							Value: "mod1",
						},
						{
							Name: "Mod 2",
							Value: "mod2",
						},
						{
							Name: "Mod 3",
							Value: "mod3",
						},
					}
                case "author":
					choices = []*discordgo.ApplicationCommandOptionChoice{
						{
							Name: "Author 1",
							Value: "author1",
						},
						{
							Name: "Author 2",
							Value: "author2",
						},
						{
							Name: "Author 3",
							Value: "author3",
						},
					}
                case "version":
					for version := range(versions) {
						choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
							Name: version,
							Value: version,
						})
					}
                }

                if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionApplicationCommandAutocompleteResult,
                    Data: &discordgo.InteractionResponseData{
                        Choices: choices,
                    },
                }); err != nil {panic(err)}
            }
        },
    }
)

func (m ModArr) Len() int {
	return len(m)
}
func (m ModArr) Swap(a, b int) {
    m[a], m[b] = m[b], m[a]
}
func (m ModArr) Less(a, b int) bool {
    a_internal := m[a].Category == "internal"
    b_internal := m[b].Category == "internal"
    if a_internal != b_internal {
        return b_internal
    }
    return m[a].Downloads_count > m[b].Downloads_count
}

func FormatVersion(input string) string {
	parts := strings.Split(input, ".")
	a, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {panic(err)}
	b, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {panic(err)}
	output := strconv.FormatInt(a, 10) + "." + strconv.FormatInt(b, 10)
	if _, ok := versions[output]; ok {
		return output
	}
	return ""
}

func CompareCache(data ModArr) {
    // updated := make(map[string]mod)
    file, err := os.ReadFile("mods.json")
    if err != nil {panic(err)}
    var cache ModArr
    if err := json.Unmarshal(file, &cache); err != nil {panic(err)}

    for i := 0; i < len(data); i++ {
        // log.Println(data[i].Name)
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

	sort.Sort(data.Results)
	for _, mod := range(data.Results) {
		if version := mod.Latest_release.Info_json.Factorio_version; version != "" {
			version = FormatVersion(version)
			if version == "" {continue}
			mods[mod.Name] = mod
			owner := strings.ToLower(mod.Owner)
			authors[owner] = append(authors[owner], mod)
			versions[version] = append(versions[version], mod)
		}
	}

    // CompareCache(data.Results)

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