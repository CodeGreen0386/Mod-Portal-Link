package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	// allMods         []Mod
	mods       map[string]*Mod
	authors    map[string]*Author
	allAuthors []*Author
	versions   map[string][]*Mod
)

type SpecificRelease struct {
	Mod     FullMod
	Release Release
}

func UpdateMods() {
	file, err := os.ReadFile("time.txt")
	if err != nil {
		log.Fatal(err)
	}
	lastUpdated := string(file)

	now := time.Now().UTC()
	defer s.UpdateCustomStatus(fmt.Sprintf("Updated: %d/%02d %d:%02d", int(now.Month()), now.Day(), now.Hour(), now.Minute()))

	modList, err := BulkRequest("https://mods.factorio.com/api/mods?page_size=max")
	if err != nil {
		log.Printf("Could not request mods: %v", err)
		return
	}

	go CacheModList(modList.Results)

	var updated []Mod
	for _, mod := range modList.Results {
		if mod.FactorioVersion() == "" {
			continue
		}
		if mod.LatestRelease.ReleasedAt < lastUpdated {
			continue
		}
		updated = append(updated, mod)
	}

	defer log.Printf("Updated %d mods\n", len(updated))

	if len(updated) == 0 {
		return
	}

	var fullMods []FullMod
	for _, mod := range updated {
		fullMod, err := mod.Request(true)
		if err == nil {
			fullMods = append(fullMods, fullMod)
		}
	}

	var releases []SpecificRelease
	for _, fullMod := range fullMods {
		for i := len(fullMod.Releases) - 1; i >= 0; i-- {
			release := fullMod.Releases[i]
			if release.ReleasedAt < lastUpdated {
				break
			}
			// fullMod.LatestRelease = release
			releases = append(releases, SpecificRelease{Mod: fullMod, Release: release})
		}
	}

	slices.SortFunc(releases, func(a, b SpecificRelease) int {
		return Ternary(a.Release.ReleasedAt <= b.Release.ReleasedAt, -1, 1)
	})

	var guildMap map[string]GuildData
	ReadJson("guilds.json", &guildMap)
	for _, guildData := range guildMap {
		if !guildData.TrackEnabled || guildData.Channel == "" {
			continue
		}
		alreadyNew := map[string]bool{}
		for _, release := range releases {
			mod := release.Mod
			isNew := mod.CreatedAt > lastUpdated && !alreadyNew[mod.Name]
			if isNew {
				alreadyNew[mod.Name] = true
			}
			if !guildData.TrackAll {
				if isNew && guildData.TrackedAuthors[mod.Owner] {
					guildData.TrackedMods[mod.Name] = true
				} else if !guildData.TrackedMods[mod.Name] {
					continue
				}
			}

			UpdateMessageSend(guildData, mod, release.Release.Version, isNew)
		}
	}

	os.WriteFile("time.txt", []byte(now.Format(time.RFC3339Nano)), 0644)
}

func UpdateMessageSend(guildData GuildData, mod FullMod, version string, isNew bool) {
	color := Ternary(isNew, colors.Green, colors.Blue)

	embed := &discordgo.MessageEmbed{
		URL:   mod.URL(),
		Title: Truncate(mod.Title, 256),
		Color: color,
		Fields: []*discordgo.MessageEmbedField{{
			Name:   "Author:",
			Value:  fmt.Sprintf("[%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
			Inline: true,
		}, {
			Name:   "Version:",
			Value:  version,
			Inline: true,
		}},
	}
	if mod.Thumbnail != "" && mod.Thumbnail != "/assets/.thumb.png" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://assets-mod.factorio.com" + mod.Thumbnail}
	}
	if guildData.Changelogs {
		changelog := mod.FormatChangelog(version)
		if changelog != "" {
			embed.Description = changelog
			embed.Fields = []*discordgo.MessageEmbedField{{
				Name:   "",
				Value:  fmt.Sprintf("**Author:** [%s](https://mods.factorio.com/user/%s)", mod.Owner, mod.Owner),
				Inline: true,
			}, {
				Name:   "",
				Value:  "**Version:** " + version,
				Inline: true,
			}}
		}
	}

	_, err := s.ChannelMessageSendEmbed(guildData.Channel, embed)
	if err != nil {
		log.Println(err)
	}
}

func CacheModList(modList []Mod) {
	// allMods = modList
	// slices.SortFunc(allMods, func(a, b Mod) int {
	// 	if a.DownloadsCount >= b.DownloadsCount {
	// 		return -1
	// 	}
	// 	return 1
	// })

	newMods := map[string]*Mod{}
	newAuthors := map[string]*Author{}
	var newAllAuthors []*Author
	newVersions := map[string][]*Mod{}

	for _, mod := range modList {
		newMods[mod.Name] = &mod
		author, ok := newAuthors[mod.Owner]
		if !ok {
			author = &Author{Name: mod.Owner}
			newAuthors[mod.Owner] = author
			newAllAuthors = append(newAllAuthors, author)
		}
		author.Mods = append(author.Mods, mod)
		author.Downloads += mod.DownloadsCount
		if version := mod.FactorioVersion(); version != "" {
			newVersions[version] = append(newVersions[version], &mod)
		}
		newVersions["all"] = append(newVersions["all"], &mod)
	}

	slices.SortFunc(newAllAuthors, func(a, b *Author) int {
		return Ternary(a.Downloads >= b.Downloads, -1, 1)
	})

	for _, modArr := range newVersions {
		slices.SortFunc(modArr, func(a, b *Mod) int {
			a_internal := a.Category == "internal"
			b_internal := b.Category == "internal"
			if a_internal != b_internal {
				return Ternary(b_internal, -1, 1)
			}
			return Ternary(a.DownloadsCount >= b.DownloadsCount, -1, 1)
		})
	}

	mods = newMods
	authors = newAuthors
	allAuthors = newAllAuthors
	versions = newVersions
}
