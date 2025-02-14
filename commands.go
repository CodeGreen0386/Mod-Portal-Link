package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func InitCommands() ([]*discordgo.ApplicationCommand, map[string]func(*discordgo.InteractionCreate, discordgo.ApplicationCommandInteractionData)) {
	var commands []*CommandData

	mod := NewCommand("mod", "Links a mod from the mod portal")
	commands = append(commands, mod)
	mod.AddOption("mod", "Mod name").SetAutocomplete()
	mod.AddOption("author", "Author filter").SetOptional().SetAutocomplete()
	mod.AddOption("version", "Factorio version filter").SetOptional().SetAutocomplete()
	mod.Handler = func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
		options := MapOptions(data.Options)
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			name := options["mod"].StringValue()
			mod := mods[name]
			if mod == nil {
				RespondError(i, "Invalid Mod Name", fmt.Sprintf("The mod %s was not found.", name))
				return
			}

			fullMod, err := mod.Request(false)
			if err != nil {
				RespondDefaultError(i)
				return
			}

			fields := []*discordgo.MessageEmbedField{{
				Value:  fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
				Inline: true,
			}, {
				Value:  fmt.Sprintf("**Downloads:** %d", mod.DownloadsCount),
				Inline: true,
			}}

			RespondEmbed(i, discordgo.MessageEmbed{
				Title:       Truncate(mod.Title, 256),
				URL:         mod.URL(),
				Description: Truncate(mod.Summary, 2048),
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: fullMod.GetThumbnail()},
				Color:       colors.Gold,
				Fields:      fields,
			})

		case discordgo.InteractionApplicationCommandAutocomplete:
			var choices []*discordgo.ApplicationCommandOptionChoice
			focused := FocusedOption(data.Options)

			switch focused.Name {
			case "mod":
				var modArr []*Mod
				if options["author"] != nil {
					modArr = versions["all"]
				} else {
					modArr = VersionFilter(options["version"])
				}
				modArr = AuthorFilter(modArr, options["author"])
				modArr = ModAutocomplete(modArr, focused.StringValue())
				choices = ModChoices(modArr)
			case "author":
				authorArr := AuthorAutocomplete(focused.StringValue())
				choices = AuthorChoices(authorArr)
			case "version":
				for version := range versions {
					choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: version, Value: version})
				}
			}

			RespondChoices(i, choices)
		}
	}

	author := NewCommand("author", "Links an author from the mod portal")
	commands = append(commands, author)
	author.AddOption("name", "Author Name").SetAutocomplete()
	author.Handler = func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
		options := MapOptions(data.Options)
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			name := options["name"].StringValue()
			author, ok := authors[name]
			if !ok {
				RespondError(i, "Invalid Author Name", fmt.Sprintf("The author `%s` was not found.", name))
				return
			}

			slices.SortFunc(author.Mods, func(a, b Mod) int {
				return Ternary(a.LatestRelease.ReleasedAt > b.LatestRelease.ReleasedAt, -1, 1)
			})

			description := "**Recent releases:**"
			for i := 0; i < 5 && i < len(author.Mods); i++ {
				mod := author.Mods[i]
				latest := mod.LatestRelease
				description += fmt.Sprintf("\n- [%s](%s) - %s - %s", mod.Title, mod.URL(), latest.Version, Timestamp(latest.ReleasedAt))
			}

			RespondEmbed(i, discordgo.MessageEmbed{
				Title:       name,
				URL:         author.URL(),
				Description: description,
				Color:       colors.Gold,
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: author.Thumbnail()},
				Fields: []*discordgo.MessageEmbedField{{
					Value:  fmt.Sprintf("**Total Mods:** %d", len(author.Mods)),
					Inline: true,
				}, {
					Value:  fmt.Sprintf("**Total Downloads:** %d", author.Downloads),
					Inline: true,
				}},
			})

		case discordgo.InteractionApplicationCommandAutocomplete:
			name := options["name"].StringValue()
			authorArr := AuthorAutocomplete(name)
			RespondChoices(i, AuthorChoices(authorArr))
		}
	}

	changelog := NewCommand("changelog", "Displays the changelog for a specific version of a mod")
	commands = append(commands, changelog)
	changelog.AddOption("mod", "Mod name").SetAutocomplete()
	changelog.AddOption("version", "Mod version").SetAutocomplete()
	changelog.Handler = func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
		options := MapOptions(data.Options)

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			value := options["mod"].StringValue()
			mod := mods[value]
			if mod == nil {
				RespondError(i, "Invalid Mod Name", fmt.Sprintf("The mod %s was not found.", value))
				return
			}

			fullMod, err := mod.Request(true)
			if err != nil {
				RespondDefaultError(i)
				return
			}

			var version string
			if options["version"] != nil {
				version = options["version"].StringValue()
			} else {
				version = mod.LatestRelease.Version
			}

			release := fullMod.GetRelease(version)
			if release == nil {
				RespondError(i, "Invalid Version", fmt.Sprintf("%s does not have a release for version `%s`.\nPlease use the autocomplete list for a valid version.", mod.Title, version))
				return
			}

			description := fullMod.FormatChangelog(version)
			if description == "" {
				description = fmt.Sprintf("No changelog for version %s", version)
			}

			RespondEmbed(i, discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s %s", Truncate(mod.Title, 256-len(version)), version),
				URL:         mod.URL(),
				Description: description,
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: fullMod.GetThumbnail()},
				Color:       colors.Gold,
				Fields: []*discordgo.MessageEmbedField{{
					Value:  fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
					Inline: true,
				}, {
					Value:  fmt.Sprintf("**Released:** %s", Timestamp(release.ReleasedAt)),
					Inline: true,
				}},
			})
		case discordgo.InteractionApplicationCommandAutocomplete:
			focused := FocusedOption(data.Options)
			var choices []*discordgo.ApplicationCommandOptionChoice

			switch focused.Name {
			case "mod":
				value := focused.StringValue()
				modArr := versions["all"]
				modArr = ModAutocomplete(modArr, value)
				modArr = VersionSort(modArr)
				choices = ModChoices(modArr)
			case "version":
				name := options["mod"]
				if name == nil {
					break
				}

				mod := mods[name.StringValue()]
				if mod == nil {
					break
				}

				fullMod, err := mod.Request(true)
				if err != nil {
					break
				}

				var versionArr []string
				for i := 1; i <= len(fullMod.Releases) && i <= 25; i++ {
					versionArr = append(versionArr, fullMod.Releases[len(fullMod.Releases)-i].Version)
				}

				choices = StringChoices(versionArr)
			}

			RespondChoices(i, choices)
		}
	}

	track := NewCommand("track", "Adds mods to the list of tracked mods").SetPermission(discordgo.PermissionManageServer)
	commands = append(commands, track)
	track.AddOption("mod", "Adds a mod to the list of tracked mods").AddOption("mod", "Mod name").SetAutocomplete()
	track.AddOption("author", "Adds an author to the list of tracked authors").AddOption("author", "Author name").SetAutocomplete()
	file := track.AddOption("file", "Adds enabled mods from a mod-list.json to the list of tracked mods")
	file.AddOption("mod-list", "mod-list.json file").SetType("file")
	track.AddOption("all", "Sets whether all mods should be tracked").AddOption("enabled", "enabled").SetType("bool")
	track.AddOption("enabled", "Sets whether update messages should be sent").AddOption("enabled", "enabled").SetType("bool")
	track.AddOption("changelogs", "Sets whether changelogs should be shown for mod updates").AddOption("enabled", "enabled").SetType("bool")
	track.AddOption("set_channel", "Sets the channel for mod updates").AddOption("channel", "The channel to send mod updates in").SetType("channel")
	track.AddOption("list", "Lists the tracked mods and authors").SetType("command")
	track.AddOption("test", "Sends a test message to the mod update channel").SetType("command")
	track.Handler = func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			subCommand := data.Options[0]
			var guildMap map[string]GuildData
			ReadJson("guilds.json", &guildMap)
			guildData := guildMap[i.GuildID]
			switch subCommand.Name {
			case "mod":
				name := subCommand.Options[0].StringValue()
				if mods[name] == nil {
					RespondError(i, "Invalid Mod Name", fmt.Sprintf("The mod `%s` does not exist. Please use the autocomplete list for a valid mod.", name))
					return
				}
				guildData.TrackedMods[name] = true
				guildData.TrackAll = false
				RespondSuccess(i, fmt.Sprintf("Added `%s` to tracked mods", name))
			case "author":
				name := subCommand.Options[0].StringValue()
				author := authors[name]
				if author == nil {
					RespondError(i, "Invalid Author Name", fmt.Sprintf("The author `%s` does not exist. Please use the autocomplete list for a valid author.", name))
					return
				}
				guildData.TrackedAuthors[name] = true
				guildData.TrackAll = false
				for _, mod := range author.Mods {
					guildData.TrackedMods[mod.Name] = true
				}
				RespondSuccess(i, fmt.Sprintf("Added `%s` to tracked authors.", name))
			case "file":
				id := subCommand.Options[0].Value.(string)
				url := data.Resolved.Attachments[id].URL
				resp, err := http.Get(url)
				if err != nil {
					RespondError(i, "Invalid Attachment", fmt.Sprintf("Could not get response from %s", url))
					return
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					RespondError(i, "Invalid Attachment", "Failed to read response body")
					return
				}
				var list ModList
				if err := json.Unmarshal(body, &list); err != nil {
					RespondError(i, "Invalid Attachment", "Failed to parse file")
					return
				}

				vanillaMods := map[string]bool{"base": true, "space-age": true, "quality": true, "elevated-rail": true}
				for _, mod := range list.Mods {
					if mod.Enabled && !vanillaMods[mod.Name] {
						guildData.TrackedMods[mod.Name] = true
					}
				}

				RespondSuccess(i, "Added enabled mods to the tracked list")
			case "all":
				value := subCommand.Options[0].BoolValue()
				guildData.TrackAll = value
				RespondSuccess(i, fmt.Sprintf("%s tracking of all mods", Ternary(value, "Enabled", "Disabled")))
			case "changelogs":
				value := subCommand.Options[0].BoolValue()
				guildData.Changelogs = value
				RespondSuccess(i, fmt.Sprintf("%s changelog updates", Ternary(value, "Enabled", "Disabled")))
			case "enabled":
				value := subCommand.Options[0].BoolValue()
				if value && guildData.Channel == "" {
					RespondError(i, "Unset Update Channel", "Please set an update channel with `/track set_channel` before enabling mod updates.")
					return
				}
				guildData.TrackEnabled = value
				RespondSuccess(i, fmt.Sprintf("%s mod update messages", Ternary(value, "Enabled", "Disabled")))
			case "set_channel":
				channel := subCommand.Options[0].ChannelValue(s)
				if channel.Type != 0 && channel.Type != 5 {
					RespondError(i, "Invalid Channel Type", fmt.Sprintf("<#%s> is not a text channel.", channel.ID))
					return
				}
				permissions, err := s.State.UserChannelPermissions(s.State.User.ID, channel.ID)
				if err != nil {
					RespondDefaultError(i)
				}
				if permissions&0x400 == 0 {
					RespondError(i, "Invalid Permissions", fmt.Sprintf("Cannot view channel <#%s>", channel.ID))
					return
				}
				if permissions&0x800 == 0 {
					RespondError(i, "Invalid Permissions", fmt.Sprintf("Cannot send messages in <#%s>", channel.ID))
					return
				}
				if permissions&0x4000 == 0 {
					RespondError(i, "Invalid Permissions", fmt.Sprintf("Cannot embed links in <#%s>", channel.ID))
					return
				}

				guildData.Channel = channel.ID
				RespondSuccess(i, fmt.Sprintf("Update channel set to <#%s>", channel.ID))
			case "list":
				if len(guildData.TrackedMods) == 0 && len(guildData.TrackedAuthors) == 0 {
					RespondSuccess(i, "No tracked mods or authors")
					return
				}

				var modArr []string
				for mod := range guildData.TrackedMods {
					modArr = append(modArr, mod)
				}
				modOut := Truncate(fmt.Sprintf("**Mods:**\n%s\n\n", strings.Join(modArr, ", ")), 2000)

				var authorArr []string
				for author := range guildData.TrackedAuthors {
					authorArr = append(authorArr, author)
				}
				authorOut := Truncate(fmt.Sprintf("**Authors:**\n%s", strings.Join(authorArr, ", ")), 2000)

				RespondSuccess(i, modOut + authorOut)
			case "test":
				_, err := s.ChannelMessageSendEmbed(guildData.Channel, &discordgo.MessageEmbed{
					Description: "Mod Update Test",
					Color:       colors.Blue,
				})
				if err != nil {
					RespondError(i, "Failed to send test mod update", "```"+err.Error()+"```")
				} else {
					RespondSuccess(i, "Mod update test successful")
				}
			}
			guildMap[i.GuildID] = guildData
			WriteJson("guilds.json", guildMap)
		case discordgo.InteractionApplicationCommandAutocomplete:
            var choices []*discordgo.ApplicationCommandOptionChoice
			focused := FocusedOption(data.Options[0].Options)
			switch focused.Name {
			case "mod":
				modArr := ModAutocomplete(versions["all"], focused.StringValue())
				modArr = VersionSort(modArr)
                choices = ModChoices(modArr)
            case "author":
                authorArr := AuthorAutocomplete(focused.StringValue())
                choices = AuthorChoices(authorArr)
			}
            RespondChoices(i, choices)
		}
	}

	untrack := NewCommand("untrack", "Removes mods from the list of tracked mods")
	commands = append(commands, untrack)
	untrack.AddOption("mod", "Removes a mod from the list of tracked mods").AddOption("mod", "Mod name").SetAutocomplete()
	untrack.AddOption("author", "Removes an author from the list of tracked authors").AddOption("author", "Author name").SetAutocomplete()
	untrack.AddOption("all", "Removes all mods and authors from both tracked lists").SetType("command")
	untrack.Handler = func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			subCommand := data.Options[0]
			var guildMap map[string]GuildData
			ReadJson("guilds.json", &guildMap)
			guildData := guildMap[i.GuildID]
			switch subCommand.Name {
			case "mod":
				name := subCommand.Options[0].StringValue()
				if mods[name] == nil {
					RespondError(i, "Invalid Mod Name", fmt.Sprintf("The mod `%s` does not exist. Please use the autocomplete list for a valid mod.", name))
                    return
				}

				delete(guildData.TrackedMods, name)
				RespondSuccess(i, fmt.Sprintf("Removed `%s` from tracked mods", name))
            case "author":
                name := subCommand.Options[0].StringValue()
                author := authors[name]
                if author == nil {
                    RespondError(i, "Invalid Author Name", fmt.Sprintf("The author `%s` does not exist. Please use the autocomplete for a valid name.", name))
                    return
                }

                for _, mod := range author.Mods {
                    delete(guildData.TrackedMods, mod.Name)
                }
                delete(guildData.TrackedAuthors, name)
                RespondSuccess(i, fmt.Sprintf("Removed `%s` from tracked authors", name))
            case "all":
                guildData.TrackedMods = map[string]bool{}
                guildData.TrackedAuthors = map[string]bool{}
                RespondSuccess(i, "Removed all mods and authors from the tracked lists")
			}
            guildMap[i.GuildID] = guildData
            WriteJson("guilds.json", guildMap)
        case discordgo.InteractionApplicationCommandAutocomplete:
            var choices []*discordgo.ApplicationCommandOptionChoice
            focused := FocusedOption(data.Options[0].Options)
            var guildMap map[string]GuildData
            ReadJson("guilds.json", &guildMap)
            guildData := guildMap[i.GuildID]
            switch focused.Name {
            case "mod":
                var modArr []*Mod
                c := 0
                for name := range guildData.TrackedMods {
                    modArr = append(modArr, mods[name])
                    if c++; c >= 25 {
                        break
                    }
                }
                choices = ModChoices(modArr)
            case "author":
                var authorArr []*Author
                c := 0
                for name := range guildData.TrackedAuthors {
                    authorArr = append(authorArr, authors[name])
                    if c++; c >= 25 {
                        break
                    }
                }
                choices = AuthorChoices(authorArr)
            }
            RespondChoices(i, choices)
		}
	}

	var retCommands []*discordgo.ApplicationCommand
	retHandlers := map[string]func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData){}
	for _, command := range commands {
		retCommands = append(retCommands, command.Compute())
		retHandlers[command.Name] = command.Handler
	}

	return retCommands, retHandlers
}

func Choice(name, value string) *discordgo.ApplicationCommandOptionChoice {
	s := strings.TrimLeft(name, " \t")
	if s == "" {
		s = value
	}
	return &discordgo.ApplicationCommandOptionChoice{
		Name:  Truncate(s, 100),
		Value: value,
	}
}

func ModChoices(modArr []*Mod) (choices []*discordgo.ApplicationCommandOptionChoice) {
	for _, mod := range modArr {
		choices = append(choices, Choice(mod.Title, mod.Name))
	}
	return choices
}

func AuthorChoices(authorArr []*Author) (choices []*discordgo.ApplicationCommandOptionChoice) {
	for _, author := range authorArr {
		choices = append(choices, Choice(author.Name, author.Name))
	}
	return choices
}

func StringChoices(sArr []string) (choices []*discordgo.ApplicationCommandOptionChoice) {
	for _, s := range sArr {
		choices = append(choices, Choice(s, s))
	}
	return choices
}

func RespondChoices(i *discordgo.InteractionCreate, choices []*discordgo.ApplicationCommandOptionChoice) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}

func RespondDefaultError(i *discordgo.InteractionCreate) {
	RespondError(i, "Process Failed", "There was a problem processing your request, please try again.")
}

func RespondError(i *discordgo.InteractionCreate, title, description string) {
	RespondEmbed(i, discordgo.MessageEmbed{
		Title:       "ERROR: " + title,
		Description: description,
		Color:       colors.Red,
	})
}

func RespondSuccess(i *discordgo.InteractionCreate, description string) {
	RespondEmbed(i, discordgo.MessageEmbed{
		Description: description,
		Color:       colors.Green,
	})
}

func RespondEmbed(i *discordgo.InteractionCreate, embed discordgo.MessageEmbed) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				&embed,
			},
		},
	})
	if err != nil {
		fmt.Printf("%v\n", err)
	}
}

func MapOptions(options []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	ret := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, option := range options {
		ret[option.Name] = option
	}
	return ret
}

func FocusedOption(options []*discordgo.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
	for _, option := range options {
		if option.Focused {
			return option
		}
	}
	return options[1]
}

type CommandData struct {
	Name        string
	Description string
	Permission  *int64
	Options     []*CommandOptionData
	Handler     func(i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData)
}

type CommandOptionData struct {
	Type         string
	Name         string
	Description  string
	Optional     bool
	Autocomplete bool
	Options      []*CommandOptionData
}

func NewCommand(name, description string) *CommandData {
	return &CommandData{
		Name:        name,
		Description: description,
	}
}

func (data *CommandData) Compute() *discordgo.ApplicationCommand {
	command := &discordgo.ApplicationCommand{
		Type:                     discordgo.ChatApplicationCommand,
		Name:                     data.Name,
		Description:              data.Description,
		DefaultMemberPermissions: data.Permission,
	}

	for _, optionData := range data.Options {
		command.Options = append(command.Options, optionData.Compute())
	}

	return command
}

func (data *CommandData) AddOption(name, description string) *CommandOptionData {
	option := &CommandOptionData{
		Name:        name,
		Description: description,
	}
	data.Options = append(data.Options, option)
	return option
}

func (data *CommandData) SetPermission(permission int64) *CommandData {
	data.Permission = &permission
	return data
}

func (data *CommandOptionData) Compute() *discordgo.ApplicationCommandOption {
	option := &discordgo.ApplicationCommandOption{
		Name:        data.Name,
		Description: data.Description,
	}
	if len(data.Options) > 0 {
		data.Type = "command"
	}
	if data.Type != "command" {
		if !data.Optional {
			option.Required = true
		}
		switch data.Type {
		case "bool":
			option.Type = discordgo.ApplicationCommandOptionBoolean
		case "file":
			option.Type = discordgo.ApplicationCommandOptionAttachment
		case "channel":
			option.Type = discordgo.ApplicationCommandOptionChannel
		default:
			option.Type = discordgo.ApplicationCommandOptionString
			option.Autocomplete = data.Autocomplete
		}
	} else {
		option.Type = discordgo.ApplicationCommandOptionSubCommand
		for _, optionData := range data.Options {
			option.Options = append(option.Options, optionData.Compute())
		}
	}

	return option
}

func (data *CommandOptionData) AddOption(name, description string) *CommandOptionData {
	option := &CommandOptionData{
		Name:        name,
		Description: description,
	}
	data.Options = append(data.Options, option)
	return option
}

func (data *CommandOptionData) SetType(optionType string) *CommandOptionData {
	data.Type = optionType
	return data
}

func (data *CommandOptionData) SetAutocomplete() *CommandOptionData {
	data.Autocomplete = true
	return data
}

func (data *CommandOptionData) SetOptional() *CommandOptionData {
	data.Optional = true
	return data
}
