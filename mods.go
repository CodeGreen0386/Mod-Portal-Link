package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Response struct {
	Pagination map[string]string `json:"pagination"`
	Results    []Mod             `json:"results"`
}

type Mod struct {
	Name           string          `json:"name"`
	Title          string          `json:"title"`
	Owner          string          `json:"owner"`
	Summary        string          `json:"summary"`
	DownloadsCount int             `json:"downloads_count"`
	Category       string          `json:"category"`
	LatestRelease  Release         `json:"latest_release"`
	Dependencies   map[string]bool `json:"dependencies"`
}

type FullMod struct {
	*Mod
	Releases  []Release `json:"releases"`
	CreatedAt string    `json:"created_at"`
	Thumbnail string    `json:"thumbnail"`
	Changelog string    `json:"changelog"`
	SourceURL string    `json:"source_url"`
}

type Release struct {
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

func (mod Mod) URL() string {
	return fmt.Sprintf("https://mods.factorio.com/mod/%s", strings.Replace(mod.Name, " ", "%20", -1))
}

func (mod Mod) FactorioVersion() string {
	version := mod.LatestRelease.InfoJson.FactorioVersion
	if version == "" {
		return ""
	}
	parts := strings.Split(version, ".")
	for i, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return ""
		}
		parts[i] = strconv.FormatInt(n, 10)
	}
	output := strings.Join(parts, ".")
	return output
}

func (mod Mod) Request(full bool) (FullMod, error) {
	url := fmt.Sprintf("https://mods.factorio.com/api/mods/%s", strings.Replace(mod.Name, " ", "%20", -1))
	if full {
		url += "/full"
	}

	var fullMod FullMod
	resp, err := http.Get(url)
	if err != nil {
		return fullMod, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fullMod, err
	}

	err = json.Unmarshal(body, &fullMod)
	fullMod.LatestRelease = mod.LatestRelease
	return fullMod, err
}

func (mod FullMod) GetThumbnail() string {
	if mod.Thumbnail == "" || mod.Thumbnail == "/assets/.thumb.png" {
		return ""
	}
	return "https://assets-mod.factorio.com" + mod.Thumbnail
}

func (mod FullMod) GetRelease(version string) *Release {
	for _, release := range mod.Releases {
		if release.Version == version {
			return &release
		}
	}
	return nil
}

func (mod FullMod) FormatChangelog(version string) string {
	parts := strings.Split(mod.Changelog, strings.Repeat("-", 99))
	if len(parts) == 1 {
		return ""
	}
	for _, part := range parts {
		part = strings.ReplaceAll(part, "\r", "")
		part = strings.ReplaceAll(part, "__", "\\__")
		i := strings.Index(part, "Version: ")
		if i == -1 {
			continue
		}
		i = i + len("Version: ")
		j := strings.Index(part[i:], "\n")
		if j == -1 {
			continue
		}
		if part[i:i+j] != version {
			break
		}
		part = part[i+j+1:]
		i = strings.Index(part, "Date: ")
		if i != -1 {
			j = strings.Index(part[i:], "\n")
			part = part[i+j+1:]
		}

		lines := strings.Split(part, "\n")
		for i, line := range lines {
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
		part = strings.Join(lines, "\n")
		re := regexp.MustCompile(`\n+`)
		part = re.ReplaceAllString(part, "\n")
		if strings.Contains(mod.SourceURL, "https://github.com/") {
			re := regexp.MustCompile(`#[0-9]+`)
			part = re.ReplaceAllStringFunc(part, func(match string) string {
				return fmt.Sprintf("[%s](%s/issues/%s)", match, mod.SourceURL, match[1:])
			})
		}
		return Truncate(part, 4096)
	}
	return ""
}

func BulkRequest(url string) (Response, error) {
	var data Response
	resp, err := http.Get(url)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return data, err
	}
	return data, nil
}

func VersionFilter(option *discordgo.ApplicationCommandInteractionDataOption) []*Mod {
	if option == nil {
		return versions[defaultVersion]
	}
	modArr, ok := versions[option.StringValue()]
	if !ok {
		return versions[defaultVersion]
	}
	return modArr
}

func AuthorFilter(modArr []*Mod, option *discordgo.ApplicationCommandInteractionDataOption) []*Mod {
	if option == nil {
		return modArr
	}
	value := option.StringValue()
	if _, ok := authors[value]; !ok {
		return modArr
	}
	var newArr []*Mod
	for _, mod := range modArr {
		if mod.Owner == value {
			newArr = append(newArr, mod)
		}
	}
	return newArr
}

func ModAutocomplete(modArr []*Mod, value string) []*Mod {
	if value == "" {
		var ret []*Mod
		for i, mod := range modArr {
			if i == 25 {
				break
			}
			ret = append(ret, mod)
		}
		return ret
	}

	value = strings.ToLower(value)
	var titleFirst, titleLast, nameFirst, nameLast []*Mod
	for _, mod := range modArr {
		if len(titleFirst) == 25 {
			break
		}

		title := strings.ToLower(mod.Title)
		i := strings.Index(title, value)
		if i == 0 {
			titleFirst = append(titleFirst, mod)
			continue
		} else if i > 0 {
			titleLast = append(titleLast, mod)
			continue
		}
		name := strings.ToLower(mod.Name)
		i = strings.Index(name, value)
		if i == 0 {
			nameFirst = append(nameFirst, mod)
			continue
		} else if i > 0 {
			nameLast = append(nameLast, mod)
			continue
		}
	}

	titleFirst = append(titleFirst, titleLast...)
	titleFirst = append(titleFirst, nameFirst...)
	titleFirst = append(titleFirst, nameLast...)

	if len(titleFirst) > 25 {
		return titleFirst[:24]
	}
	return titleFirst
}

func VersionSort(modArr []*Mod) []*Mod {
	slices.SortStableFunc(modArr, func(a, b *Mod) int {
		aV, bV := a.FactorioVersion(), b.FactorioVersion()
		if aV > bV {
			return -1
		} else if aV < bV {
			return 1
		}
		return 0
	})
	return modArr
}