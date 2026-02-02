package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/gocolly/colly/v2"
	"github.com/spf13/cobra"
)

const baseUrl = "https://guides.gsh-servers.com/path-of-titans/guides/curve-overrides/alderons/"
const fileName = "./dinos.json"

type DinoStats map[string]any

func main() {
	rootCmd := &cobra.Command{Use: "pot"}

	runCmd := &cobra.Command{
		Use: "find [name]",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(args)
			qry := args[0]
			d, err := load()
			if err != nil {
				log.Fatal(err)
			}

			qryDinos := dinoSearch(qry, d)
			for k := range qryDinos {

				/* TODO Create search and format data */
				fmt.Println(k)
			}
		},
	}

	scrapeCmd := &cobra.Command{
		Use:   "scrape",
		Short: "Scrape links from a webpage",
		Run: func(cmd *cobra.Command, args []string) {
			println("Scraping...")
			scrape()

		},
	}

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(scrapeCmd)

	runCmd.Flags().StringP("dino", "d", "", "Dino query")

	rootCmd.Execute()
}

func dinoSearch(query string, dj DinoMap) DinoStats {

	qds := DinoStats{}

	lc := strings.ToLower(query)

	for k := range dj {
		if strings.Contains(strings.ToLower(k), lc) {
			qds[k] = dj[k]
		}
	}

	return qds
}

/* WebScraping */

type DinoMap map[string]Dino

type Dino struct {
	Name  string                       `json:"name"`
	URL   string                       `json:"url"`
	Stats map[string]map[string]string `json:"stats"`
}

func scrape() {

	var dinoMap = DinoMap{}

	list := colly.NewCollector()
	stats := list.Clone()

	list.OnHTML("main h3 a", func(e *colly.HTMLElement) {
		name := e.Text
		url := e.Request.AbsoluteURL(e.Attr("href"))

		ctx := colly.NewContext()
		ctx.Put("name", name)
		ctx.Put("url", url)

		stats.Request("GET", url, nil, ctx, nil)
	})

	stats.OnHTML("main span.line", func(h *colly.HTMLElement) {

		name := h.Request.Ctx.Get("name")
		url := h.Request.Ctx.Get("url")

		// fmt.Println(name)

		cat, nm, val, err := parseStatLine(h.Text)
		if err != nil {
			log.Fatal(err)
		}

		d, ok := dinoMap[name]
		if !ok {
			d = Dino{
				Name:  name,
				URL:   url,
				Stats: map[string]map[string]string{},
			}
		}

		if d.Stats[cat] == nil {
			d.Stats[cat] = map[string]string{}
		}
		d.Stats[cat][nm] = val

		dinoMap[name] = d
	})

	list.Visit(baseUrl)
	list.Wait()
	stats.Wait()

	save(dinoMap)

}

var capture = regexp.MustCompile(`\(([^)]+)\)`)

func parseStatLine(l string) (string, string, string, error) {

	parts := strings.Split(l, `"`)
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("bad line: %s", l)
	}

	cat := parts[0]
	raw := parts[1]

	match := capture.FindStringSubmatch(raw)
	if len(match) < 2 {
		return "", "", "", fmt.Errorf("no values found: %s", l)
	}

	values := match[1]

	var statCat, statName string

	if strings.Contains(cat, ".") {
		s := strings.SplitN(cat, ".", 2)
		statCat = s[0]
		statName = s[1]
	} else {
		statCat = "Combat"
		statName = cat
	}

	return statCat, statName, values, nil
}

/* IO */

func save(dm DinoMap) {

	b, err := json.Marshal(dm)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(fileName, b, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func load() (DinoMap, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll((f))
	if err != nil {
		return nil, err
	}
	var m DinoMap
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
