package main

import (
	"io"
	"net/http"
	"strings"
)

type Author struct {
	Name      string
	Mods      []Mod
	Downloads int
}

func (author Author) Thumbnail() string {
	url := "https://mods.factorio.com/user/" + author.Name

	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	content := string(html)
	i := strings.Index(content, "author-card-thumbnail")
	i += strings.Index(content[i:], "src=\"") + 5
	j := i + strings.Index(content[i:], "\"")

	thumbnail := content[i:j]
	if thumbnail == "/static/no-avatar.png" {
		return ""
	}
	return thumbnail
}

func (author Author) URL() string {
	return "https://mods.factorio.com/user/" + author.Name
}

func AuthorAutocomplete(value string) []*Author {
	if value == "" {
		var authorArr []*Author
		for i, author := range allAuthors {
			if i == 25 {
				break
			}
			authorArr = append(authorArr, author)
		}
		return authorArr
	}

	value = strings.ToLower(value)
	var first, last []*Author
	for _, author := range allAuthors {
		name := strings.ToLower(author.Name)
		i := strings.Index(name, value)
		if i == 0 {
			first = append(first, author)
		} else if i > 0 {
			last = append(last, author)
		}

		if len(first) == 25 {
			break
		}
	}

	first = append(first, last...)
	if len(first) > 25 {
		return first[:24]
	}
	return first
}
