package main

import (
	"feedgen/pkg/generators"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	var feedType, outputType, outputFile, configFile, workDir string
	var gen generators.Generator
	flag.StringVar(&feedType, "f", "", "Provide feed type to generate. One of: css, h1")
	flag.StringVar(&outputType, "t", "rss", "Type of the output feed: rss, atom, or json")
	flag.StringVar(&outputFile, "o", "feed.xml", "Output file")
	flag.StringVar(&workDir, "w", "./workdir", "Work dir, need by some feed generators to store state")
	flag.StringVar(&configFile, "c", "", "Config file for CSS generator.")
	flag.Parse()

	switch feedType {
	case "css":
		if configFile == "" {
			flag.Usage()
			os.Exit(1)
		}
		g, err := generators.NewCssGeneratorFromJson(configFile)
		gen = &g
		if err != nil {
			log.Fatal(err.Error())
		}
	case "h1":
		gen = &generators.H1Generator{}
	case "p0":
		g := generators.ProjectZeroGenerator{}
		g.WorkDir(workDir)
		gen = &g
	default:
		log.Println("Missing valid feed type")
		flag.Usage()
		os.Exit(1)
	}

	feed, err := gen.Feed()
	if err != nil {
		log.Println(err.Error())
	}

	if outputFile == "-" || outputFile == "" {
		switch strings.ToLower(outputType) {
		case "rss":
			out, _ := feed.ToRss()
			fmt.Print(out)
		case "atom":
			out, _ := feed.ToAtom()
			fmt.Print(out)
		case "json":
			out, _ := feed.ToJSON()
			fmt.Print(out)
		}
	} else {
		fp, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err.Error())
		}
		defer fp.Close()

		switch strings.ToLower(outputType) {
		case "rss":
			feed.WriteRss(fp)
		case "atom":
			feed.WriteAtom(fp)
		case "json":
			feed.WriteJSON(fp)
		}
		log.Printf("Wrote feed to %s\n", outputFile)
	}
}