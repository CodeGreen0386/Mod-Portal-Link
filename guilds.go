package main

import "github.com/bwmarrin/discordgo"

type GuildData struct {
	Channel        string          `json:"channel"`
	Changelogs     bool            `json:"changelogs"`
	TrackEnabled   bool            `json:"track_enabled"`
	TrackAll       bool            `json:"track_all"`
	TrackedMods    map[string]bool `json:"tracked_mods"`
	TrackedAuthors map[string]bool `json:"tracked_authors"`
}

func GuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	var guildMap map[string]GuildData
	ReadJson("guilds.json", &guildMap)
	guildData, ok := guildMap[g.ID]
	if !ok {
		guildData.TrackedAuthors = map[string]bool{}
		guildData.TrackedMods = map[string]bool{}
	}
	guildMap[g.ID] = guildData
	WriteJson("guilds.json", guildMap)
}