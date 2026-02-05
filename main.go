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

const curveUrl = "https://guides.gsh-servers.com/path-of-titans/guides/curve-overrides/alderons/"
const fandomUrl = "https://path-of-titans.fandom.com/wiki/Dinosaur_Stats"
const curveFileName = "./c_dinos.json"
const fandomFileName = "./f_dinos.json"

type DinoMap map[string]Dino

type Stat map[string]string

type Dino struct {
	Name  string          `json:"name"`
	URL   string          `json:"url"`
	Stats map[string]Stat `json:"stats"`
}

func main() {
	rootCmd := &cobra.Command{Use: "pot"}

	findCmd := &cobra.Command{
		Use:   "find [name] [stat]",
		Args:  cobra.RangeArgs(1, 2),
		Short: "Search dino stats",
		Long: `Search dinosaur stats. 
		Stat values are listed per growth stage:
		Hatchling, Juvenile, Adolescent Sub-Adult, Adult
		Example:
		Armor: 1,1,1,1,1
`,
		Run: runFind,
	}

	findCmd.Flags().BoolP("ability", "a", false, "Ability stats")
	findCmd.Flags().BoolP("multiplier", "m", false, "Multiplier stats")
	findCmd.Flags().BoolP("core", "c", false, "Core stats")

	scrapeCmd := &cobra.Command{
		Use:   "scrape",
		Args:  cobra.NoArgs,
		Short: "Scrape links from a webpage",
		Run:   runScrape,
	}

	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(scrapeCmd)

	rootCmd.Execute()
}

/* Commands */

func runFind(cmd *cobra.Command, args []string) {
	dinoName := args[0]

	d, err := load()
	if err != nil {
		log.Fatal(err)
	}

	qryDinos := dinoSearch(dinoName, d)

	ability, _ := cmd.Flags().GetBool("ability")
	core, _ := cmd.Flags().GetBool("core")
	mult, _ := cmd.Flags().GetBool("multiplier")

	var statQry string
	if len(args) > 1 {
		statQry = args[1]
		qryDinos = statSearch(statQry, qryDinos)
	}

	//Filter fields if flags are provided
	if ability || core || mult {
		for _, d := range qryDinos {
			for cat := range d.Stats {
				switch cat {
				case "Combat":
					{
						if !ability {
							delete(d.Stats, cat)
						}
					}
				case "Core":
					{
						if !core {
							delete(d.Stats, cat)
						}
					}
				case "Multiplier":
					{
						if !mult {
							delete(d.Stats, cat)
						}
					}
				}
			}
		}
	}

	print(cmd, qryDinos)
}

func runScrape(cmd *cobra.Command, args []string) {
	fmt.Println("Scraping...")
	// scrapeCurve()
	scrapeFandom()
	fmt.Println("Scraping complete!")
}

/* Query Fns */

func dinoSearch(query string, dm DinoMap) DinoMap {

	qds := DinoMap{}

	lc := strings.ToLower(query)

	for k := range dm {
		if strings.Contains(strings.ToLower(k), lc) {
			qds[k] = dm[k]
		}
	}

	return qds
}

func statSearch(qry string, dm DinoMap) DinoMap {
	out := DinoMap{}

	qry = strings.ToLower(qry)

	for nm, d := range dm {
		extStats := map[string]Stat{}

		for cat, inner := range d.Stats {
			for stat, val := range inner {
				if strings.Contains(strings.ToLower(cat+"."+stat), qry) {
					if extStats[cat] == nil {
						extStats[cat] = map[string]string{}
					}
					extStats[cat][stat] = val
				}
			}
		}

		if len(extStats) > 0 {
			d.Stats = extStats
			out[nm] = d
		}
	}

	return out
}

/* WebScraping */

type FandomShape struct {
	Title string     `json:"title"`
	Cats  []string   `json:"cats"`
	Data  [][]string `json:"data"`
}

func scrapeFandom() {
	list := colly.NewCollector()

	fsMap := map[string]FandomShape{}

	list.OnHTML("table", func(e *colly.HTMLElement) {

		title := e.ChildText("caption")
		if title == "" {
			return
		}
		hdrs := e.ChildTexts("th")

		fields := e.ChildTexts("td")

		for i := range fields {
			println(i, fields[i])
		}

		data := [][]string{}

		temp := []string{}

		for i := range fields {
			if i%len(hdrs) == 0 && i != 0 {
				data = append(data, temp)
				temp = []string{}
			}
			temp = append(temp, fields[i])
		}
		if len(temp) > 0 {
			data = append(data, temp)
		}

		println(len(hdrs), len(data[0]))

		fs := FandomShape{
			Title: title,
			Cats:  hdrs,
			Data:  data,
		}

		fsMap[fs.Title] = fs
	})

	list.Visit(fandomUrl)
	list.Wait()

	fmt.Println(fsMap)

	dm := mapFandomDinos(fsMap)

	save(dm, fandomFileName)

}

func scrapeCurve() error {

	var dinoMap = DinoMap{}
	var scrapeErr error

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

		cat, nm, val, err := parseStatLine(h.Text)
		if err != nil {
			scrapeErr = err
			h.Request.Abort()
			return
		}

		d, ok := dinoMap[name]
		if !ok {
			d = Dino{
				Name:  name,
				URL:   url,
				Stats: map[string]Stat{},
			}
		}

		if d.Stats[cat] == nil {
			d.Stats[cat] = map[string]string{}
		}
		d.Stats[cat][nm] = val

		dinoMap[name] = d
	})

	list.Visit(curveUrl)
	list.Wait()
	stats.Wait()

	if scrapeErr != nil {
		return scrapeErr
	}

	save(dinoMap, curveFileName)

	return nil
}

/* Data Mapping */

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

func mapFandomDinos(dinos map[string]FandomShape) DinoMap {
	dm := DinoMap{}

	for catKey, fs := range dinos {
		hdrs := fs.Cats[1:]
		for _, row := range fs.Data {

			name := row[0]
			fields := row[1:]

			d, ok := dm[name]
			if !ok {
				d = Dino{
					Name:  name,
					URL:   "",
					Stats: map[string]Stat{},
				}
			}

			st, ok := d.Stats[catKey]
			if !ok {
				st = Stat{}
			}

			for i, h := range hdrs {

				st[h] = fields[i]
			}

			d.Stats[catKey] = st

			dm[name] = d
		}
	}

	return dm
}

/* Persistence */

func save(dm DinoMap, fn string) {

	b, err := json.Marshal(dm)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(fn, b, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func load() (DinoMap, error) {
	f, err := os.Open(curveFileName)
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

/* out */

func print(cmd *cobra.Command, dm DinoMap) {
	b, _ := json.MarshalIndent(dm, "", "  ")
	cmd.Println(string(b))
}
