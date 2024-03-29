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
    "regexp"
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
    Name 		   string          `json:"name"`
    Title 		   string          `json:"title"`
    Owner 		   string          `json:"owner"`
    Summary 	   string          `json:"summary"`
    DownloadsCount int             `json:"downloads_count"`
    Category       string          `json:"category"`
    LatestRelease  LatestRelease   `json:"latest_release"`
    Dependencies   map[string]bool `json:"dependencies"`
}

type FullMod struct {
    *Mod
    Releases  []Release `json:"releases"`
    Thumbnail string    `json:"thumbnail"`
    Changelog string    `json:"changelog"`
    SourceURL string    `json:"source_url"`
}

type Release struct {
    *LatestRelease
}

type LatestRelease struct {
    InfoJson   InfoJson `json:"info_json"`
    ReleasedAt string   `json:"released_at"`
    Version    string   `json:"version"`
}

type InfoJson struct {
    FactorioVersion string   `json:"factorio_version"`
    Dependencies    []string `json:"dependencies"`
}

type ModList struct {
    Mods []ModListMod `json:"mods"`
}

type ModListMod struct {
    Name    string `json:"name"`
    Enabled bool   `json:"enabled"`
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
    Aqua:   0x1abc9c,
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
    for {
        file, err := os.ReadFile(filename)
        if err != nil {panic(err)}
        if err := json.Unmarshal(file, v); err == nil {
            return
        } else {
            log.Printf("Bad JSON read of %s, retrying...", filename)
            time.Sleep(time.Second * 5)
        }
    }
}

func WriteJson(filename string, v any) {
    file, err := json.MarshalIndent(v, "", "    ")
    if err != nil {panic(err)}
    os.WriteFile(filename, file, 0644)
}

func ModURL(name string) string {
    return fmt.Sprintf("https://mods.factorio.com/mod/%s", strings.Replace(name, " ", "%20", -1))
}

func RequestMod(name string, data *FullMod, full bool) error {
    url := fmt.Sprintf("https://mods.factorio.com/api/mods/%s", strings.Replace(name, " ", "%20", -1))
    if full {url = url + "/full"}

    resp, err := http.Get(url)
    if err != nil {return err}
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {return err}

    if err := json.Unmarshal(body, &data); err != nil {panic(err)}
    return nil
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
    if _, ok := authors[author]; !ok {return modArr}
    newList := make(ModArr, 0)
    for _, mod := range(modArr) {
        if mod.Owner == author {
            newList = append(newList, mod)
        }
    }
    return newList
}

func Truncate(s string, max int) string {
    if len(s) > max {
        return s[0:max-3] + "..."
    }
    return s
}

func NewChoice(name, value string) *discordgo.ApplicationCommandOptionChoice{
    s := strings.TrimLeft(name, " \t")
    if s == "" {
        s = value
    }
    return &discordgo.ApplicationCommandOptionChoice{
        Name: Truncate(s, 100),
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
    value = strings.ToLower(value)
    titleFirst := []*discordgo.ApplicationCommandOptionChoice{}
    titleLast  := []*discordgo.ApplicationCommandOptionChoice{}
    nameFirst  := []*discordgo.ApplicationCommandOptionChoice{}
    nameLast   := []*discordgo.ApplicationCommandOptionChoice{}
    for _, mod := range(modArr) {
        if len(titleFirst) == 25 {break}
        if value == "" {
            titleFirst = append(titleFirst, NewChoice(mod.Title, mod.Name))
            continue
        }

        title, name := strings.ToLower(mod.Title), strings.ToLower(mod.Name)
        titleIndex := strings.Index(title, value)
        if titleIndex >= 0 {
            newChoice := NewChoice(mod.Title, mod.Name)
            if titleIndex == 0 {
                titleFirst = append(titleFirst, newChoice)
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
    value = strings.ToLower(value)
    authorFirst := []*discordgo.ApplicationCommandOptionChoice{}
    authorLast  := []*discordgo.ApplicationCommandOptionChoice{}
    for author := range(authors) {
        if len(authorFirst) == 25 {break}
        if value == "" {
            authorFirst = append(authorFirst, NewChoice(author, author))
            continue
        }

        name := strings.ToLower(author)
        nameIndex := strings.Index(name, value)
        if nameIndex >= 0 {
            newChoice := NewChoice(author, author)
            if nameIndex == 0 {
                authorFirst = append(authorFirst, newChoice)
            } else {
                authorLast = append(authorLast, newChoice)
            }
        }
    }

    authorFirst = MaxCombine(authorFirst, authorLast, 25)
    return authorFirst
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

func RespondDefaultError(s *discordgo.Session, i *discordgo.InteractionCreate) {
    RespondEmbed(s, i, &discordgo.MessageEmbed{
        Title: "ERROR: Process Failed",
        Description: "There was an error processing your request. Please try again!",
        Color: colors.Red,
    })
    return
}

func ISOtoUnix(iso string) int64 {
    timeValue, _ := time.Parse(time.RFC3339Nano, iso)
    return timeValue.Unix()
}

func commands() []*discordgo.ApplicationCommand {
    manageServer := int64(32)
    return []*discordgo.ApplicationCommand{{ // mod
        Type: discordgo.ChatApplicationCommand,
        Name: "mod",
        Description: "Links a mod from the mod portal",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "mod",
            Description: "Mod name",
            Required: true,
            Autocomplete: true,
        }, {
            Type: discordgo.ApplicationCommandOptionString,
            Name: "author",
            Description: "Author filter",
            Autocomplete: true,
        }, {
            Type: discordgo.ApplicationCommandOptionString,
            Name: "version",
            Description: "Version filter",
            Autocomplete: true,
        }},
    }, { // changelog
        Type: discordgo.ChatApplicationCommand,
        Name: "changelog",
        Description: "Displays the changelog of a mod",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "mod",
            Description: "Mod name",
            Required: true,
            Autocomplete: true,
        // }, {
        //     Type: discordgo.ApplicationCommandOptionString,
        //     Name: "version",
        //     Description: "Version filter",
        //     Autocomplete: true,
        }},
    }, { // dependencies
        Type: discordgo.ChatApplicationCommand,
        Name: "dependencies",
        Description: "Links all mods that depend on another mod",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "mod",
            Description: "Mod name",
            Required: true,
            Autocomplete: true,
        }},
    }, { // author
        Type: discordgo.ChatApplicationCommand,
        Name: "author",
        Description: "Links an author from the mod portal",
        Options: []*discordgo.ApplicationCommandOption{{
            Type: discordgo.ApplicationCommandOptionString,
            Name: "author",
            Description: "Author name",
            Required: true,
            Autocomplete: true,
        }},
    }, { // track
        Type: discordgo.ChatApplicationCommand,
        Name: "track",
        Description: "Adds mods to the list of tracked mods",
        DefaultMemberPermissions: &manageServer,
        Options: []*discordgo.ApplicationCommandOption{{ // mod
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
        }, { // author
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
        }, { // file
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "file",
            Description: "Adds enabled mods from a mod-list.json to the list of tracked mods",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionAttachment,
                Name: "mod-list",
                Description: "mod-list.json file",
                Required: true,
            }},
        }, { // all
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "all",
            Description: "Sets whether all mods should be tracked",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionBoolean,
                Name: "enabled",
                Description: "enabled",
                Required: true,
            }},
        }, { // enabled
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "enabled",
            Description: "Sets whether mod update notifications should be enabled",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionBoolean,
                Name: "enabled",
                Description: "enabled",
                Required: true,
            }},
        }, { // changelogs
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "changelogs",
            Description: "Sets whether the changelog should be shown for mod update messages",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionBoolean,
                Name: "enabled",
                Description: "enabled",
                Required: true,
            }},
        }, { // set_channel
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "set_channel",
            Description: "Sets the channel for mod update notifications",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionChannel,
                Name: "channel",
                Description: "The channel to send update notifications to",
                Required: true,
            }},
        }, { // list
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "list",
            Description: "Lists the tracked mods and authors",
        }, { // test
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "test",
            Description: "Tests the mod update channel with an embed",
        }},
    }, { // untrack
        Type: discordgo.ChatApplicationCommand,
        Name: "untrack",
        Description: "Removes mods from the list of tracked mods",
        DefaultMemberPermissions: &manageServer,
        Options: []*discordgo.ApplicationCommandOption{{ // mod
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
        }, { // author
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "author",
            Description: "Removes an author from the list of tracked authors",
            Options: []*discordgo.ApplicationCommandOption{{
                Type: discordgo.ApplicationCommandOptionString,
                Name: "author",
                Description: "Author name",
                Required: true,
                Autocomplete: true,
            }},
        }, { // all
            Type: discordgo.ApplicationCommandOptionSubCommand,
            Name: "all",
            Description: "Removes everything from the tracked list",
        }},
    }}
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
    "mod": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        options := OptionsMap(&data)

        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            value := options["mod"].StringValue()
            mod, ok := mods[value]
            if !ok {
                version := "1.1"
                if option, ok := options["version"]; ok {
                    version = option.StringValue()
                }

                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Title: "ERROR: Invalid Mod Name",
                    Description: fmt.Sprintf("The mod `%s` was not found.\nVersion searched: `%s`", value, version),
                    Color: colors.Red,
                })
                return
            }

            var resp FullMod
            var thumbnail string
            err := RequestMod(value, &resp, false)
            if err != nil {
                RespondDefaultError(s, i)
                return
            }
            if resp.Thumbnail != "" && resp.Thumbnail != "/assets/.thumb.png" {
                thumbnail = "https://assets-mod.factorio.com" + resp.Thumbnail
            }

            fields := []*discordgo.MessageEmbedField{{
                Name: "",
                Value: fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
                Inline: true,
            }, {
                Name: "",
                Value: fmt.Sprintf("**Downloads:** %d", mod.DownloadsCount),
                Inline: true,
            }}

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: Truncate(mod.Title, 256),
                URL: ModURL(mod.Name),
                Description: Truncate(mod.Summary, 2048),
                Thumbnail: &discordgo.MessageEmbedThumbnail{URL: thumbnail},
                Color: colors.Gold,
                Fields: fields,
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options)
            if err != nil {panic(err)}

            switch focused.Name {
            case "mod":
                value := focused.StringValue()
                modArr := VersionFilter(&data, options)
                modArr = AuthorFilter(modArr, &data, options)
                choices = ModAutocomplete(modArr, value)
            case "author":
                value := focused.StringValue()
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
    "changelog": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        options := OptionsMap(&data)
        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            value := options["mod"].StringValue()
            mod, ok := mods[value]
            if !ok {
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Title: "ERROR: Invalid Mod Name",
                    Description: fmt.Sprintf("The mod `%s` was not found.", value),
                    Color: colors.Red,
                })
                return
            }

            var resp FullMod
            var thumbnail string
            err := RequestMod(value, &resp, true)
            if err != nil {
                RespondDefaultError(s, i)
                return
            }
            if resp.Thumbnail != "" && resp.Thumbnail != "/assets/.thumb.png" {
                thumbnail = "https://assets-mod.factorio.com" + resp.Thumbnail
            }
            description := FormatChangelog(resp, mod)
            if description == "" {
                description = fmt.Sprintf("No changelog for latest release.")
            }

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: Truncate(mod.Title, 256),
                URL: ModURL(mod.Name),
                Description: description,
                Thumbnail: &discordgo.MessageEmbedThumbnail{URL: thumbnail},
                Color: colors.Gold,
                Fields: []*discordgo.MessageEmbedField{{
                    Name: "",
                    Value: fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
                    Inline: true,
                }, {
                    Name: "",
                    Value: fmt.Sprintf("**Version:** %s", mod.LatestRelease.Version),
                    Inline: true,
                }},
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options)
            if err != nil {panic(err)}

            switch focused.Name {
            case "mod":
                value := focused.StringValue()
                modArr := VersionFilter(&data, options)
                modArr = AuthorFilter(modArr, &data, options)
                choices = ModAutocomplete(modArr, value)
            }

            RespondChoices(s, i, choices)
        }
    },
    "author": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        data := i.ApplicationCommandData()
        options := OptionsMap(&data)

        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            value := options["author"].StringValue()

            authorMods, ok := authors[value]
            if !ok {
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Title: "ERROR: Invalid Author Name",
                    Description: fmt.Sprintf("The author `%s` was not found.", value),
                    Color: colors.Red,
                })
                return
            }

            URL := fmt.Sprintf("https://mods.factorio.com/user/%s", value)

            resp, err := http.Get(URL)
            if err != nil {
                RespondDefaultError(s, i)
                return
            }
            defer resp.Body.Close()
            html, err := io.ReadAll(resp.Body)
            if err != nil {
                RespondDefaultError(s, i)
                return
            }
            content := string(html)
            index := strings.Index(content, "profile-image-dropzone")
            index += strings.Index(content[index:], "https")
            index2 := index + strings.Index(content[index:], "\n") - 2
            thumbnail := content[index:index2]

            downloads := 0
            for _, mod := range(authorMods) {
                downloads += mod.DownloadsCount
            }

            modTotal := len(authorMods)
            recentMods := make(RecentMods, modTotal)
            copy(recentMods, authorMods)
            sort.Sort(recentMods)

            description := "**Recent releases:**"
            if modTotal > 5 {
                modTotal = 5
            }

            for i := 0; i < modTotal; i++ {
                mod := recentMods[i]
                timestamp := ""
                if mod.LatestRelease.ReleasedAt != "" {
                    timestamp = fmt.Sprintf("<t:%d:R>", ISOtoUnix(mod.LatestRelease.ReleasedAt))
                }
                description += fmt.Sprintf("\n- [%s](%s) - %s - %s", mod.Title, ModURL(mod.Name), mod.LatestRelease.Version, timestamp)
            }

            RespondEmbed(s, i, &discordgo.MessageEmbed{
                Title: value,
                URL: URL,
                Description: description,
                Thumbnail: &discordgo.MessageEmbedThumbnail{URL: thumbnail},
                Color: colors.Gold,
                Fields: []*discordgo.MessageEmbedField{{
                    Name: "",
                    Value: fmt.Sprintf("**Total Mods:** %d", len(authorMods)),
                    Inline: true,
                }, {
                    Name: "",
                    Value: fmt.Sprintf("**Total Downloads:** %d", downloads),
                    Inline: true,

                }},
            })
        case discordgo.InteractionApplicationCommandAutocomplete:
            value := options["author"].StringValue()
            choices := AuthorAutocomplete(value)
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
                value := subCommand.Options[0].StringValue()
                if _, ok := mods[value]; !ok {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Mod Name",
                        Description: fmt.Sprintf("The mod `%s` does not exist. Please use the autocomplete for a valid name.", value),
                        Color: colors.Red,
                    })
                    return
                }
                guildData.TrackedMods[value] = true
                guildData.TrackAll = false
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Added `%s` to tracked mods", value),
                    Color: colors.Green,
                })
            case "author":
                value := subCommand.Options[0].StringValue()
                if _, ok := authors[value]; !ok {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Author Name",
                        Description: fmt.Sprintf("The author `%s` does not exist. Please use the autocomplete for a valid name.", value),
                        Color: colors.Red,
                    })
                    return
                }
                guildData.TrackedAuthors[value] = true
                guildData.TrackAll = false
                for _, mod := range(authors[value]) {
                    guildData.TrackedMods[mod.Name] = true
                }
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Added `%s` to tracked authors", value),
                    Color: colors.Green,
                })
            case "file":
                attachmentID := subCommand.Options[0].Value.(string)
                attachmentURL := data.Resolved.Attachments[attachmentID].URL
                resp, err := http.Get(attachmentURL)
                if err != nil {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Response",
                        Description: fmt.Sprintf("Could not get response from %s", attachmentURL),
                        Color: colors.Red,
                    })
                    return
                }

                defer resp.Body.Close()
                body, err := io.ReadAll(resp.Body)
                if err != nil {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Body",
                        Description: "Failed to read response body",
                        Color: colors.Red,
                    })
                    return
                }

                var list ModList
                if err := json.Unmarshal(body, &list); err != nil {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid File",
                        Description: "Failed to process file",
                        Color: colors.Red,
                    })
                    return
                }

                for _, mod := range(list.Mods) {
                    if mod.Enabled && mod.Name != "base" {
                        guildData.TrackedMods[mod.Name] = true
                    }
                }

                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Added enabled mods to the tracked list"),
                    Color: colors.Green,
                })
            case "all":
                value := subCommand.Options[0].BoolValue()
                guildData.TrackAll = value

                var output string
                if value {
                    output = "Enabled"
                } else {
                    output = "Disabled"
                }

                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("%s tracking of all mods", output),
                    Color: colors.Green,
                })
            case "enabled":
                value := subCommand.Options[0].BoolValue()
                if value && guildData.Channel == "" {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: No Update Channel",
                        Description: "Please set an update channel with `/setchannel` before enabling mod update notifications.",
                        Color: colors.Red,
                    })
                    return
                }

                guildData.TrackEnabled = value

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
            case "set_channel":
                channel := subCommand.Options[0].ChannelValue(s)
                if channel.Type != 0 && channel.Type != 5 {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Channel Type",
                        Description: fmt.Sprintf("<#%s> is not a text channel.", channel.ID),
                        Color: colors.Red,
                    })
                    return
                }

                permissions, err := s.State.UserChannelPermissions(s.State.User.ID, channel.ID)
                if err != nil {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Unexpected error",
                        Description: "`" + err.Error() + "`",
                        Color: colors.Red,
                    })
                    return
                }

                if permissions & 0x400 == 0 {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Permission",
                        Description: fmt.Sprintf("Cannot view channel <#%s>", channel.ID),
                        Color: colors.Red,
                    })
                    return
                }

                if permissions & 0x800 == 0 {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Permission",
                        Description: fmt.Sprintf("Cannot send messages in <#%s>", channel.ID),
                        Color: colors.Red,
                    })
                    return
                }

                if permissions & 0x4000 == 0 {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Permission",
                        Description: fmt.Sprintf("Cannot embed links in <#%s>", channel.ID),
                        Color: colors.Red,
                    })
                    return
                }

                guildData.Channel = channel.ID
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Update channel set to <#%s>", channel.ID),
                    Color: colors.Green,
                })
            case "changelogs":
                value := subCommand.Options[0].BoolValue()
                guildData.Changelogs = value

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
            case "list":
                if len(guildData.TrackedMods) == 0 && len(guildData.TrackedAuthors) == 0 {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Description: "No tracked mods or authors",
                        Color: colors.Green,
                    })
                    return
                }
                modArr := make([]string, 0)
                for mod := range(guildData.TrackedMods) {
                    modArr = append(modArr, mod)
                }

                authorArr := make([]string, 0)
                for author := range(guildData.TrackedAuthors) {
                    authorArr = append(authorArr, author)
                }

                modOut := strings.Join(modArr, ", ")
                authorOut := strings.Join(authorArr, ", ")

                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("**Mods:**\n%s\n\n**Authors:**\n%s", modOut, authorOut),
                    Color: colors.Gold,
                })
            case "test":
                embed := &discordgo.MessageEmbed{
                    Description: "Mod Update Test",
                    Color: colors.Green,
                }
                _, err := s.ChannelMessageSendEmbed(guildData.Channel, embed)
                if err != nil {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Failed to send test mod update",
                        Description: "```" + err.Error() + "```",
                        Color: colors.Red,
                    })
                } else {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Description: "Mod update test successful",
                        Color: colors.Green,
                    })
                }
            }
            guildMap[i.GuildID] = guildData
            WriteJson("guilds.json", guildMap)
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options[0].Options)
            if err != nil {panic(err)}
            switch focused.Name {
            case "mod":
                value := focused.StringValue()
                choices = ModAutocomplete(versions[defaultVersion], value)
            case "author":
                value := focused.StringValue()
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
                value := subCommand.Options[0].StringValue()
                if _, ok := mods[value]; !ok {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Mod Name",
                        Description: fmt.Sprintf("The mod `%s` does not exist. Please use the autocomplete for a valid name.", value),
                        Color: colors.Red,
                    })
                    return
                }
                if _, ok := guildData.TrackedMods[value]; ok {
                    delete(guildData.TrackedMods, value)
                }
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Removed `%s` from tracked mods", value),
                    Color: colors.Green,
                })
            case "author":
                value := subCommand.Options[0].StringValue()
                if _, ok := authors[value]; !ok {
                    RespondEmbed(s, i, &discordgo.MessageEmbed{
                        Title: "ERROR: Invalid Author Name",
                        Description: fmt.Sprintf("The author `%s` does not exist. Please use the autocomplete for a valid name.", value),
                        Color: colors.Red,
                    })
                    return
                }
                guildData.TrackedAuthors[value] = true
                for _, mod := range(authors[value]) {
                    if _, ok := guildData.TrackedMods[mod.Name]; ok{
                        delete(guildData.TrackedMods, mod.Name)
                    }
                }
                if _, ok := guildData.TrackedAuthors[value]; ok {
                    delete(guildData.TrackedAuthors, value)
                }
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: fmt.Sprintf("Removed `%s` from tracked authors", value),
                    Color: colors.Green,
                })
            case "all":
                for k := range(guildData.TrackedMods) {
                    delete(guildData.TrackedMods, k)
                }
                for k := range(guildData.TrackedAuthors) {
                    delete(guildData.TrackedAuthors, k)
                }
                RespondEmbed(s, i, &discordgo.MessageEmbed{
                    Description: "Removed all mods and authors from the tracked list",
                    Color: colors.Green,
                })
            }
            guildMap[i.GuildID] = guildData
            WriteJson("guilds.json", guildMap)
        case discordgo.InteractionApplicationCommandAutocomplete:
            choices := []*discordgo.ApplicationCommandOptionChoice{}
            focused, err := FocusedOption(data.Options[0].Options)
            if err != nil {panic(err)}
            switch focused.Name {
            case "mod":
                value := focused.StringValue()
                choices = ModAutocomplete(versions[defaultVersion], value)
            case "author":
                value := focused.StringValue()
                choices = AuthorAutocomplete(value)
            }
            RespondChoices(s, i, choices)
        }
    },
}

func (m ModArr) Len() int {return len(m)}
func (m ModArr) Swap(a, b int) {m[a], m[b] = m[b], m[a]}
func (m ModArr) Less(a, b int) bool {
    a_internal := m[a].Category == "internal"
    b_internal := m[b].Category == "internal"
    if a_internal != b_internal {
        return b_internal
    }
    return m[a].DownloadsCount > m[b].DownloadsCount
}

type UpdatedMods []Mod
func (m UpdatedMods) Len() int {return len(m)}
func (m UpdatedMods) Swap(a, b int) {m[a], m[b] = m[b], m[a]}
func (m UpdatedMods) Less(a, b int) bool {
    return m[a].LatestRelease.ReleasedAt < m[b].LatestRelease.ReleasedAt
}

type RecentMods []Mod
func (m RecentMods) Len() int {return len(m)}
func (m RecentMods) Swap(a, b int) {m[a], m[b] = m[b], m[a]}
func (m RecentMods) Less(a, b int) bool {
    return m[a].LatestRelease.ReleasedAt > m[b].LatestRelease.ReleasedAt
}

func FormatVersion(input string) string {
    if input == "" {return ""}
    parts := strings.Split(input, ".")
    for i, part := range(parts) {
        n, err := strconv.ParseInt(part, 10, 64)
        if err != nil {panic(err)}
        parts[i] = strconv.FormatInt(n, 10)
    }
    output := strings.Join(parts, ".")
    if _, ok := versions[output]; ok {
        return output
    }
    return ""
}

func UpdatedMod(a, b Mod) bool {
    aParts := strings.Split(a.LatestRelease.Version, ".")
    bParts := strings.Split(b.LatestRelease.Version, ".")
    for i := 0; i < len(aParts); i++ {
        aInt, _ := strconv.ParseInt(aParts[i], 10, 64)
        bInt, _ := strconv.ParseInt(bParts[i], 10, 64)
        if aInt < bInt {
            return true
        }
    }
    return false
}

func CacheMods(modArr ModArr) {
    mods = make(map[string]Mod)
    authors = make(map[string]ModArr)
    versions = make(map[string]ModArr)
    for _, version := range(versionList) {
        versions[version] = make(ModArr, 0)
    }

    for _, mod := range(modArr) {
        version := FormatVersion(mod.LatestRelease.InfoJson.FactorioVersion)
        if version == "" {continue}
        mods[mod.Name] = mod
        author := mod.Owner
        authors[author] = append(authors[author], mod)
        versions[version] = append(versions[version], mod)
    }
}

func FormatChangelog(resp FullMod, mod Mod) string {
    parts := strings.Split(resp.Changelog, strings.Repeat("-", 99))
    if len(parts) == 1 {return ""}
    changelog := parts[1]
    if changelog == resp.Changelog {return ""}
    changelog = strings.ReplaceAll(changelog, "\r", "")
    changelog = strings.ReplaceAll(changelog, "__", "\\__")
    index := strings.Index(changelog, "Version: ")
    if index == -1 {return ""}
    index = index + len("Version: ")
    endIndex := strings.Index(changelog[index:], "\n")
    if endIndex == -1 {return ""}
    version := changelog[index:index+endIndex]
    if version != mod.LatestRelease.Version {return ""}
    changelog = changelog[index+endIndex+1:]
    index = strings.Index(changelog, "Date: ")
    if index != -1 {
        endIndex = strings.Index(changelog[index:], "\n")
        changelog = changelog[index+endIndex+1:]
    }
    lines := strings.Split(changelog, "\n")
    for i, line := range(lines) {
        if strings.TrimSpace(line) == "" {
            lines[i] = ""
            continue
        }
        re := regexp.MustCompile(`^\s{0,4}`)
        line = re.ReplaceAllString(line, "")
        l := len(line)
        if line[:1] != " " && line[l-1:] == ":" {
            line = "**" + line + "**"
        }
        lines[i] = line
    }
    changelog = strings.Join(lines, "\n")
    re := regexp.MustCompile(`\n+`)
    changelog = re.ReplaceAllString(changelog, "\n")
    if strings.Contains(resp.SourceURL, "https://github.com/") {
        re := regexp.MustCompile(`#[0-9]+`)
        changelog = re.ReplaceAllStringFunc(changelog, func(match string) string {
            return fmt.Sprintf("[%s](%s/issues/%s)", match, resp.SourceURL, match[1:])
        })
    }
    return Truncate(changelog, 4096)
}

func UpdateMessageSend(s *discordgo.Session, guildData GuildData, mod Mod, resp FullMod, isNew bool) {
    var color int
    if isNew {
        color = colors.Green
    } else {
        color = colors.Blue
    }

    embed := &discordgo.MessageEmbed{
        URL: ModURL(mod.Name),
        Title: Truncate(mod.Title, 256),
        Color: color,
        Fields: []*discordgo.MessageEmbedField{{
            Name: "Author:",
            Value: fmt.Sprintf("[%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
            Inline: true,
        }, {
            Name: "Version:",
            Value: mod.LatestRelease.Version,
            Inline: true,
        }},
    }

    if resp.Thumbnail != "" && resp.Thumbnail != "/assets/.thumb.png" {
        embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://assets-mod.factorio.com" + resp.Thumbnail}
    }
    if guildData.Changelogs {
        changelog := FormatChangelog(resp, mod)
        if changelog != "" {
            embed.Description = changelog
            embed.Fields = []*discordgo.MessageEmbedField{{
                Name: "",
                Value: fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
                Inline: true,
            }, {
                Name: "",
                Value: "**Version:** " + mod.LatestRelease.Version,
                Inline: true,
            }}
        }
    }

    _, err := s.ChannelMessageSendEmbed(guildData.Channel, embed)
    if err != nil {
        log.Println(err)
    }
}

func CompareCache(modArr ModArr) {
    updated := make(UpdatedMods, 0)
    oldMods := make(map[string]Mod)
    newMods := make(map[string]bool)
    var cache ModArr
    ReadJson("mods.json", &cache)

    for _, mod := range(modArr) {
        version := FormatVersion(mod.LatestRelease.InfoJson.FactorioVersion)
        if version == "" {continue}
        if oldMod, ok := mods[mod.Name]; ok {
            if !UpdatedMod(oldMod, mod) {
                continue
            }
            oldMods[oldMod.Name] = oldMod
        } else {
            newMods[mod.Name] = true
        }
        updated = append(updated, mod)
    }
    if updated.Len() == 0 {return}
    sort.Sort(updated)

    fullMods := make(map[string]FullMod)
    for _, mod := range(updated) {
        var resp FullMod
        RequestMod(mod.Name, &resp, true)
        fullMods[mod.Name] = resp
        dependencies := make(map[string]bool)
        for _, dependency := range(resp.Releases[len(resp.Releases)].InfoJson.Dependencies) {
            dependencies[strings.Split(dependency, " ")[1]] = true
        }
        for _, dependency := range(mod.LatestRelease.InfoJson.Dependencies) {
            if _, ok := dependencies[dependency]; !ok {
                delete(mods[dependency].Dependencies, mod.Name)
            }
        }
        for name := range(dependencies) {
            mods[name].Dependencies[mod.Name] = true
        }
    }

    var guildMap GuildMap
    ReadJson("guilds.json", &guildMap)
    for _, guildData := range(guildMap) {
        if !guildData.TrackEnabled {continue}
        if guildData.Channel == "" {continue}
        for _, mod := range(updated) {
            _, ok := newMods[mod.Name]
            if ok {
                if !guildData.TrackAll {
                    if _, author := guildData.TrackedAuthors[mod.Owner]; author {
                        guildData.TrackedMods[(mod.Name)] = true
                    } else {
                        continue
                    }
                }
            } else {
                _, tracked := guildData.TrackedMods[mod.Name]
                if !(guildData.TrackAll || tracked) {
                    continue
                }
            }

            UpdateMessageSend(s, guildData, mod, fullMods[mod.Name], ok)
        }
    }
    WriteJson("guilds.json", guildMap)
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
    if err := json.Unmarshal(body, &data); err != nil {
        log.Println(err)
        return
    }
    modArr := data.Results
    sort.Sort(modArr)

    CompareCache(modArr)
    CacheMods(modArr)
    WriteJson("mods.json", modArr)
    log.Println("Updated mods.json")
}

func ReadCache() {
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

    var modArr ModArr
    ReadJson("mods.json", &modArr)
    CacheMods(modArr)
    log.Println("Read mods.json")
}

func main() {
    s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {log.Println("READY")})
    s.AddHandler(func(s *discordgo.Session, g *discordgo.GuildCreate) {
        var guildMap GuildMap
        ReadJson("guilds.json", &guildMap)
        guildData, ok := guildMap[g.ID]
        if !ok {
            guildData.TrackedAuthors = make(map[string]bool)
            guildData.TrackedMods = make(map[string]bool)
        }
        guildMap[g.ID] = guildData
        WriteJson("guilds.json", guildMap)
    })
    s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
            h(s, i)
        }
    })
    if err := s.Open(); err != nil {
        log.Fatalf("Cannot open session: %v", err)
    }
    defer s.Close()

    createdCommands, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands())
    if err != nil {
        log.Fatalf("Cannot register commands: %v", err)
    }

    ReadCache()
    go func() {
        for {
            UpdateCache()
            time.Sleep(time.Minute * 5)
        }
    }()

    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt)
    <-stop
    log.Println("Shutting down...")

    for _, cmd := range(createdCommands) {
        if err := s.ApplicationCommandDelete(s.State.User.ID, "", cmd.ID); err != nil {
            log.Fatalf("Cannot delete command %q: %v", cmd.Name, err)
        }
    }
}

// TODO: /changelog version
// TODO: /api
// TODO: /mod required by