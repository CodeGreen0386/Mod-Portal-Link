package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

func ReadJson(filename string, v any) {
	c := 0
	for {
		file, err := os.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(file, v); err == nil {
			return
		} else {
			log.Printf("Bad JSON read of %s, retrying...", filename)
			c++
			if c == 3 {
				panic(err)
			}
			time.Sleep(time.Second)
		}
	}
}

func WriteJson(filename string, v any) {
    file, err := json.MarshalIndent(v, "", "    ")
    if err != nil {panic(err)}
    os.WriteFile(filename, file, 0644)
}