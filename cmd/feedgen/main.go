package main

import (
	"feedgen/pkg/generators"
	"flag"
	"fmt"
	"github.com/gorilla/feeds"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func chooseType(feedType string, configFile string, workDir string ) generators.Generator{
	var gen generators.Generator
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
		err := g.WorkDir(workDir)
		if err != nil {
			return nil
		}

		// Replace the Created timestamp with the updated time
		// This way newly disclosed issues appear
		g.RegisterItemModifier(func(item *feeds.Item) {
			item.Created = item.Updated
		})
		gen = &g
	case "p0rca":
		g := generators.ProjectZeroRCAGenerator{}
		err := g.WorkDir(workDir)
		if err != nil {
			return nil
		}
		gen = &g
	case "syzbot":
		g := generators.SyzbotGenerator{}
		err := g.WorkDir(workDir)
		if err != nil {
			return nil
		}
		gen = &g
	default:
		log.Println("Missing valid feed type")
		flag.Usage()
		os.Exit(1)
	}

	return gen
}

func handle(gen generators.Generator, outputName string, outputType string) {
	feed, err := gen.Feed()
	if err != nil {
		log.Fatalln(err.Error())
	}

	if _, err := os.Stat(outputName); !os.IsNotExist(err) {
		if err := os.Remove(outputName); err != nil {
			log.Fatal(err)
		}
	}

	fp, err := os.OpenFile(outputName, os.O_WRONLY|os.O_CREATE, 0644)
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
	log.Printf("Wrote feed to %s\n", outputName)
}

func main() {
	/*
	* PARAMETERS
	*/
	var feedType, outputType, outputFile, configFile, workDir, outputDir string
	var configDir = "./configs/"

	flag.StringVar(&feedType, "f", "all", "Provide feed type to generate. One of: css, h1, p0, p0rca, syzbot")
	flag.StringVar(&outputType, "t", "rss", "Type of the output feed: rss, atom, or json")
	flag.StringVar(&outputFile, "o", "feed.xml", "Output file")
	flag.StringVar(&outputDir, "oD", "./rss_output", "Output file")
	flag.StringVar(&workDir, "w", "./workdir", "Work dir, need by some feed generators to store state")
	flag.StringVar(&configFile, "c", "", "Config file for CSS generator.")
	flag.Parse()

	/*
	* RE-GENERATE ALL
	*/
	var genMap = make(map[string] generators.Generator)

	if feedType == "all"{
		var gen generators.Generator

		listFeedTypes := [] string{
			"h1", "p0", "p0rca", "syzbot",
		}

		for _, provider := range listFeedTypes {
			fmt.Printf("Generate rss from provider: %s\n", provider)
			gen = chooseType(provider, "", workDir)
			// genList = append(genList, gen)
			genMap[provider] = gen
		}

		configFiles, err := ioutil.ReadDir(configDir)
		if err != nil {
			log.Fatal(err)
		}
		for _, configFile := range configFiles {
			fullPathName := fmt.Sprintf("%s/%s", configDir, configFile.Name())
			fmt.Printf("Generate rss from config file: %s\n", fullPathName)
			exportName := configFile.Name()[:len(configFile.Name()) - 5] // remove .json from filename

			genMap[exportName] = chooseType("css", fullPathName, "")
		}

	}else {
		genMap[feedType] = chooseType(feedType, configFile, workDir)
	}

	/*
	* HANDLER
	*/
	for outputFile, gen := range genMap {
		fullPathName := fmt.Sprintf("%s/%s.%s", outputDir, outputFile, outputType)
		handle(gen, fullPathName, outputType)
	}



}
