package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

type GuildMap map[string]GuildData
type GuildData struct {
    Channel        string          `json:"channel"`
    Changelogs     bool            `json:"changelogs"`
    TrackEnabled   bool            `json:"track_enabled"`
    TrackAll       bool            `json:"track_all"`
    TrackedMods    map[string]bool `json:"tracked_mods"`
    TrackedAuthors map[string]bool `json:"tracked_authors"`
}

type Response struct {
    Pagination map[string]string `json:"pagination"`
    Results    ModArr            `json:"results"`
}

type ModArr []Mod
type Mod struct {
    Name 		   string        `json:"name"`
    Title 		   string        `json:"title"`
    Owner 		   string        `json:"owner"`
    Summary 	   string        `json:"summary"`
    DownloadsCount int64         `json:"downloads_count"`
    Category       string        `json:"category"`
    LatestRelease  LatestRelease `json:"latest_release"`
}

type FullMod struct {
    *Mod
    Thumbnail string
}

type LatestRelease struct {
    InfoJson InfoJson `json:"info_json"`
}

type InfoJson struct {
    FactorioVersion string `json:"factorio_version"`
}

var (
    s        *discordgo.Session
    mods     map[string]Mod
    authors  map[string]ModArr
    versions map[string]ModArr
)

var versionList = [...]string{"1.1", "1.0", "0.18", "0.17", "0.16", "0.15", "0.14", "0.13"}
const defaultVersion = "1.1"

type Colors struct {
    Aqua   int
    Green  int
    Blue   int
    Purple int
    Pink   int
    Gold   int
    Orange int
    Red    int
    Gray   int
}

var colors = Colors{
    Aqua:   0x1abc,
    Green:  0x57f287,
    Blue:   0x3498db,
    Purple: 0x9b59b6,
    Pink:   0xe91e63,
    Gold:   0xf1c40f,
    Orange: 0x367322,
    Red:    0xed4245,
    Gray:   0x95a5a6,
}

func init() {
    file, err := os.ReadFile("token.txt")
    if err != nil {log.Fatal(err)}
    token := string(file)
    s, err = discordgo.New("Bot " + token)
    if err != nil {log.Fatalf("Invalid token: %v", token)}

    mods = make(map[string]Mod)
    authors = make(map[string]ModArr)
    versions = make(map[string]ModArr)
    for _, version := range(versionList) {
        versions[version] = make(ModArr, 0)
    }
}

func ReadJson(filename string, v any) {
    file, err := os.ReadFile(filename)
    if err != nil {panic(err)}
    if err := json.Unmarshal(file, v); err != nil {panic(err)}
}

func WriteJson(filename string, v any) {
    file, err := json.MarshalIndent(v, "", "    ")
    if err != nil {panic(err)}
    os.WriteFile(filename, file, 0644)
}

func FormatRequestName(name string) string {
    return strings.Replace(name, " ", "%20", -1)
}

func RequestMod(name string, data *FullMod, full bool) {
    name = FormatRequestName(name)
    url := "https://mods.factorio.com/api/mods/%s"
    if full {url = url + "/full"}

    resp, err := http.Get(fmt.Sprintf(url, name))
    if err != nil {panic(err)}
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {panic(err)}

    if err := json.Unmarshal(body, &data); err != nil {panic(err)}
}

func VersionFilter(data *discordgo.ApplicationCommandInteractionData, options map[string]*discordgo.ApplicationCommandInteractionDataOption) ModArr {
    option, ok := options["version"]
    if !ok {return versions[defaultVersion]}
    version := option.StringValue()
    if list, ok := versions[version]; ok {
        return list
    }
    return versions[defaultVersion]
}

func AuthorFilter(modArr ModArr, data *discordgo.ApplicationCommandInteractionData, options map[string]*discordgo.ApplicationCommandInteractionDataOption) ModArr {
    option, ok := options["author"]
    if !ok {return modArr}
    author := option.StringValue()
    if author == "" {return modArr}
    author = strings.ToLower(author)
    if _, ok := authors[author]; !ok {return modArr}
    newList := make(ModArr, 0)
    for _, mod := range modArr {
        if strings.ToLower(mod.Owner) == author {
            newList = append(newList, mod)
        }
    }
    return newList
}

func OptionsMap(data *discordgo.ApplicationCommandInteractionData) map[string]*discordgo.ApplicationCommandInteractionDataOption {
    options := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(data.Options))
    for _, option := range(data.Options) {
        options[option.Name] = option
    }
    return options
}

func FocusedOption(options []*discordgo.ApplicationCommandInteractionDataOption) (*discordgo.ApplicationCommandInteractionDataOption, error) {
    for _, option := range(options) {
        if option.Focused {
            return option, nil
        }
    }
    return nil, errors.New("No focused option found")
}

func NewChoice(name, value string) *discordgo.ApplicationCommandOptionChoice{
    if len(name) > 100 {
        name = name[0:97] + "..."
    }
    return &discordgo.ApplicationCommandOptionChoice{
        Name: name,
        Value: value,
    }
}

func MaxCombine(a, b []*discordgo.ApplicationCommandOptionChoice, max int) []*discordgo.ApplicationCommandOptionChoice {
    for _, choice := range(b) {
        if len(a) == max {return a}
        a = append(a, choice)
    }
    return a
}

func ModAutocomplete(modArr ModArr, value string) []*discordgo.ApplicationCommandOptionChoice {
    titleFirst := []*discordgo.ApplicationCommandOptionChoice{}
    titleLast  := []*discordgo.ApplicationCommandOptionChoice{}
    nameFirst  := []*discordgo.ApplicationCommandOptionChoice{}
    nameLast   := []*discordgo.ApplicationCommandOptionChoice{}
    for _, mod := range(modArr) {
        title, name := strings.ToLower(mod.Title), strings.ToLower(mod.Name)
        if value == "" {
            titleFirst = append(titleFirst, NewChoice(mod.Title, mod.Name))
            if len(titleFirst) == 25 {break}
            continue
        }

        titleIndex := strings.Index(title, value)
        if titleIndex >= 0 {
            newChoice := NewChoice(mod.Title, mod.Name)
            if titleIndex == 0 {
                titleFirst = append(titleFirst, newChoice)
                if len(titleFirst) == 25 {break}
            } else {
                titleLast = append(titleLast, newChoice)
            }
            continue
        }

        nameIndex := strings.Index(name, value)
        if nameIndex >= 0 {
            newChoice := NewChoice(mod.Title, mod.Name)
            if nameIndex == 0 {
                nameFirst = append(nameFirst, newChoice)
            } else {
                nameLast = append(nameLast, newChoice)
            }
        }
    }

    titleFirst = MaxCombine(titleFirst, titleLast, 25)
    titleFirst = MaxCombine(titleFirst, nameFirst, 25)
    titleFirst = MaxCombine(titleFirst, nameLast, 25)
    return titleFirst
}

func AuthorAutocomplete(value string) []*discordgo.ApplicationCommandOptionChoice {
    choices := []*discordgo.ApplicationCommandOptionChoice{}
    for author, mods := range(authors) {
        if len(choices) == 25 {break}
        if value == "" || strings.Contains(author, value) {
            choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
                Name: mods[0].Owner,
                Value: author,
            })
        }
    }
    return choices
}

func RespondChoices(s *discordgo.Session, i *discordgo.InteractionCreate, choices []*discordgo.ApplicationCommandOptionChoice) {
    if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionApplicationCommandAutocompleteResult,
        Data: &discordgo.InteractionResponseData{
            Choices: choices,
        },
    }); err != nil {panic(err)}
}

func RespondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Embeds: []*discordgo.MessageEmbed{
                embed,
            },
        },
    })
}

var commands = []*discordgo.ApplicationCommand{
    {
        Type: discordgo.ChatApplicationCommand,
        Name: "mod",
        Description: "Links a mod from the mod portal",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "mod",
            Description: "Mod name",
            Required: true,
            Autocomplete: true,
        },{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "author",
            Description: "Author filter",
            Autocomplete: true,
        },{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "version",
            Description: "Version filter",
            Autocomplete: true,
        }},
    },{
        Type: discordgo.ChatApplicationCommand,
        Name: "track",
        Description: "Adds mods to the list of tracked mods",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "mod",
            Description: "Adds a mod to the list of tracked mods",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionString,
                Name: "mod",
                Description: "Mod name",
                Required: true,
                Autocomplete: true,
            }},
        },{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "author",
            Description: "Adds an author to the list of tracked authors",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionString,
                Name: "author",
                Description: "Author name",
                Required: true,
                Autocomplete: true,
            }},
        },{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "all",
            Description: "Adds all mods to the tracked list and enables mod update tracking",
        }},
    },{
        Type: discordgo.ChatApplicationCommand,
        Name: "untrack",
        Description: "Removes mods from the list of tracked mods",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "mod",
            Description: "Removes a mod from the list of tracked mods",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionString,
                Name: "mod",
                Description: "Mod name",
                Required: true,
                Autocomplete: true,
            }},
        },{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "author",
            Description: "Removes an author from the list of tracked authors",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionString,
                Name: "author",
                Description: "Author name",
                Required: true,
            }},
        },{
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "all",
            Description: "Removes everything from the tracked list and disables mod update tracking",
        }},
    },{
        Type: discordgo.ChatApplicationCommand,
        Name: "set_channel",
        Description: "sets the channel for mod update notifications",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionChannel,
            Name: "channel",
            Description: "The channel to send update notifications to",
            Required: true,
        }},
    },{
        Type: discordgo.ChatApplicationCommand,
        Name: "changelogs",
        Description: "Sets whether the changelog should be shown for mod update messages",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionBoolean,
            Name: "enabled",
            Description: "enabled",
            Required: true,
        }},
    },{
        Type: discordgo.ChatApplicationCommand,
        Name: "updates",
        Description: "Sets whether mod update notifications should be enabled",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionBoolean,
            Name: "enabled",
            Description: "enabled",
            Required: true,
        }},
    },
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
    "mod": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        options := OptionsMap(&data)

        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            name := options["mod"].StringValue()
            mod, ok := mods[name]
            if !ok {
                version := "1.1"
                if option, ok := options["version"]; ok {
                    version = option.StringValue()
                }

                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Title: "ERROR: Invalid Mod Name",
                    Description: fmt.Sprintf("The mod `%s` does not exist.\nVersion searched: `%s`", name, version),
                    Color: colors.Red,
                })
                return
            }

            var resp FullMod
            var thumbnail string
            RequestMod(name, &resp, false)
            if resp.Thumbnail != "" && resp.Thumbnail != "/assets/.thumb.png" {
                thumbnail = "https://assets-mod.factorio.com/" + resp.Thumbnail
            }

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                URL: fmt.Sprintf("https://mods.factorio.com/mod/%s", FormatRequestName(name)),
                Title: mod.Title,
                Description: mod.Summary,
                Thumbnail: &discordgo.MessageEmbedThumbnail{URL: thumbnail},
                Color: colors.Gold,
                Fields: []*discordgo.MessageEmbedField{
                    {
                        Name: "Author:",
                        Value: fmt.Sprintf("`%s`", mod.Owner),
                        Inline: true,
                    },
                    {
                        Name: "Downloads:",
                        Value: fmt.Sprintf("`%s`", strconv.FormatInt(mod.DownloadsCount, 10)),
                        Inline: true,
                    },
                },
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options)
            if err != nil {panic(err)}

            switch focused.Name {
            case "mod":
                value := strings.ToLower(focused.StringValue())
                modArr := VersionFilter(&data, options)
                modArr = AuthorFilter(modArr, &data, options)
                choices = ModAutocomplete(modArr, value)
            case "author":
                value := strings.ToLower(focused.StringValue())
                choices = AuthorAutocomplete(value)
            case "version":
                for _, version := range(versionList) {
                    choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
                        Name: version,
                        Value: version,
                    })
                }
            }
            RespondChoices(s, i, choices)
        }
    },
    "track": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            subCommand := data.Options[0]
            var guildMap GuildMap
            ReadJson("guilds.json", &guildMap)
            guildData := guildMap[i.GuildID]
            switch subCommand.Name {
            case "mod":
                value := strings.ToLower(subCommand.Options[0].StringValue())
                guildData.TrackedMods[value] = true
            case "author":
                value := strings.ToLower(subCommand.Options[0].StringValue())
                guildData.TrackedMods[value] = true
            case "all":
                guildData.TrackAll = true
                guildData.TrackEnabled = true
            }
            guildMap[i.GuildID] = guildData
            WriteJson("guilds.json", guildMap)

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: "Test Response",
                Description: "burg",
                Color: colors.Green,
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options[0].Options)
            if err != nil {panic(err)}
            switch focused.Name {
            case "mod":
                value := strings.ToLower(focused.StringValue())
                choices = ModAutocomplete(versions[defaultVersion], value)
            case "author":
                value := strings.ToLower(focused.StringValue())
                choices = AuthorAutocomplete(value)
            }
            RespondChoices(s, i, choices)
        }
    },
    "untrack": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            subCommand := data.Options[0]
            var guildMap GuildMap
            ReadJson("guilds.json", &guildMap)
            guildData := guildMap[i.GuildID]
            switch subCommand.Name {
            case "mod":
                value := strings.ToLower(subCommand.Options[0].StringValue())
                if _, ok := guildData.TrackedMods[value]; ok {
                    delete(guildData.TrackedMods, value)
                }
            case "author":
                value := strings.ToLower(subCommand.Options[0].StringValue())
                if _, ok := guildData.TrackedAuthors[value]; ok {
                    delete(guildData.TrackedAuthors, value)
                }
            case "all":
                guildData.TrackAll = false
                guildData.TrackEnabled = false
                for k := range(guildData.TrackedMods) {
                    delete(guildData.TrackedMods, k)
                }
                for k := range(guildData.TrackedAuthors) {
                    delete(guildData.TrackedAuthors, k)
                }
            }
            guildMap[i.GuildID] = guildData
            WriteJson("guilds.json", guildMap)

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: "Untrack",
                Description: subCommand.Options[0].Name,
                Color: colors.Green,
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options[0].Options)
            if err != nil {panic(err)}
            switch focused.Name {
            case "mod":
                value := strings.ToLower(focused.StringValue())
                choices = ModAutocomplete(versions[defaultVersion], value)
            case "author":
                value := strings.ToLower(focused.StringValue())
                choices = AuthorAutocomplete(value)
            }
            RespondChoices(s, i, choices)
        }
    },
    "set_channel": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        channel := data.Options[0].ChannelValue(s)
        if channel.Type != 0 {
            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: "ERROR: Invalid Channel Type",
                Description: fmt.Sprintf("`%s` is not a text channel.", channel.Name),
                Color: colors.Red,
            })
            return
        }
        var guildMap GuildMap
        ReadJson("guilds.json", &guildMap)
        guildData := guildMap[i.GuildID]
        guildData.Channel = channel.ID
        guildMap[i.GuildID] = guildData
        WriteJson("guilds.json", guildMap)

        RespondEmbed(s, i, &discordgo.MessageEmbed{
            Description: fmt.Sprintf("Update channel set to <#%s>", channel.ID),
            Color: colors.Green,
        })
    },
    "changelogs": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        value := data.Options[0].BoolValue()

        var guildMap GuildMap
        ReadJson("guilds.json", &guildMap)
        guildData := guildMap[i.GuildID]
        guildData.Changelogs = value
        guildMap[i.GuildID] = guildData
        WriteJson("guilds.json", guildMap)

        var output string
        if value {
            output = "Enabled"
        } else {
            output = "Disabled"
        }

        RespondEmbed(s, i, &discordgo.MessageEmbed{
            Description: fmt.Sprintf("%s changelog updates", output),
            Color: colors.Green,
        })
    },
    "updates": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        value := data.Options[0].BoolValue()

        var guildMap GuildMap
        ReadJson("guilds.json", &guildMap)
        guildData := guildMap[i.GuildID]
        guildData.TrackEnabled = value
        guildMap[i.GuildID] = guildData
        WriteJson("guilds.json", guildMap)

        var output string
        if value {
            output = "Enabled"
        } else {
            output = "Disabled"
        }

        RespondEmbed(s, i, &discordgo.MessageEmbed{
            Description: fmt.Sprintf("%s mod update notifications", output),
            Color: colors.Green,
        })
    },
}

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
    return m[a].DownloadsCount > m[b].DownloadsCount
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
    var cache ModArr
    ReadJson("mods.json", &cache)

    // var guilds GuildData
    // ReadJson("guilds.json", guilds)
    // for _, mod := range(cache) {

    // }
}

func UpdateCache() {
    resp, err := http.Get("https://mods.factorio.com/api/mods?page_size=max")
    if err != nil {
        log.Printf("Failed to update mods.json:\n%s", err)
        return
    }
    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Failed to update mods.json:\n%s", err)
        return
    }

    var data Response
    if err := json.Unmarshal(body, &data); err != nil {panic(err)}

    sort.Sort(data.Results)
    for _, mod := range(data.Results) {
        if version := mod.LatestRelease.InfoJson.FactorioVersion; version != "" {
            version = FormatVersion(version)
            if version == "" {continue}
            mods[mod.Name] = mod
            owner := strings.ToLower(mod.Owner)
            authors[owner] = append(authors[owner], mod)
            versions[version] = append(versions[version], mod)
        }
    }

    // CompareCache(data.Results)
    WriteJson("mods.json", data.Results)

    log.Println("Updated mods.json")
}

func ReadCache() {
    var modArr ModArr
    ReadJson("mods.json", &modArr)
    for _, mod := range(modArr) {
        if version := mod.LatestRelease.InfoJson.FactorioVersion; version != "" {
            version = FormatVersion(version)
            if version == "" {continue}
            mods[mod.Name] = mod
            owner := strings.ToLower(mod.Owner)
            authors[owner] = append(authors[owner], mod)
            versions[version] = append(versions[version], mod)
        }
    }
    log.Println("Read mods.json")
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

    if false {
        go func() {
            for {
                UpdateCache()
                time.Sleep(time.Minute * 5)
            }
        }()
    } else {
        ReadCache()
    }

    var guildMap GuildMap
    ReadJson("guilds.json", &guildMap)
    for _, guild := range(s.State.Ready.Guilds) {
        guildData, ok := guildMap[guild.ID]
        if !ok {
            guildData.TrackedAuthors = make(map[string]bool)
            guildData.TrackedMods = make(map[string]bool)
        }
        guildMap[guild.ID] = guildData
    }
    WriteJson("guilds.json", guildMap)

    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt)
    <-stop
    log.Println("Shutting down...")

    for _, cmd := range createdCommands {
        if err := s.ApplicationCommandDelete(s.State.User.ID, "", cmd.ID); err != nil {
            log.Fatalf("Cannot delete command %q: %v", cmd.Name, err)
        }
    }
}