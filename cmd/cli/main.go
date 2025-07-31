package main

import (
	"flag"
	"log"
	"os"

	"notion-blog/internal"
	notion_blog "notion-blog/pkg"

	"github.com/itzg/go-flagsfiller"
	"github.com/joho/godotenv"
)

var config notion_blog.BlogConfig

func parseFlagsConfig() {
	// create a FlagSetFiller
	filler := flagsfiller.New()
	// fill and map struct fields to flags
	err := filler.Fill(flag.CommandLine, &config)
	if err != nil {
		log.Fatal(err)
	}

	// parse command-line like usual
	flag.Parse()
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file provided")
	}
	// check that the environment contains NOTION_SECRET
	if os.Getenv("NOTION_SECRET") == "" {
		log.Fatal("NOTION_SECRET environment variable is not set")
	}

	log.Println("Parsing command-line flags...")
	parseFlagsConfig()

	log.Printf("Config: %+v", config)
	log.Printf("Starting Notion Blog generation on database id %s...", config.DatabaseID)
	if err := internal.ParseAndGenerate(config); err != nil {
		log.Fatal(err)
	}
}
