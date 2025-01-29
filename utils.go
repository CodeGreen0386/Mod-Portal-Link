package main

import (
	"fmt"
	"time"
)

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

func Truncate(s string, max int) string {
	if len(s) > max {
		return s[0:max-3] + "..."
	}
	return s
}

func Ternary[T any](a bool, b T, c T) T {
	if a {
		return b
	}
	return c
}

func Timestamp(t string) string {
	timestamp, _ := time.Parse(time.RFC3339Nano, t)
	return fmt.Sprintf("<t:%d:R>", timestamp.Unix())
}