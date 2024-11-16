package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/briandowns/spinner"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/manifoldco/promptui"
	"github.com/otiai10/gosseract/v2"
	"github.com/spf13/cobra"
)

var ACCEPTED_FILES = []string{"png", "jpg", "jpeg"}

type Doc struct {
	ID       int    `storm:"id,increment"`
	Filename string `storm:"index"`
	Text     string
}

func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	return home
}

func getScreenshotDir() string {
	screenshortDir := os.Getenv("SCREENSHOT_DIR")

	if screenshortDir == "" {
		return fmt.Sprintf("%s/%s", getHomeDir(), "screenshots")
	}

	return screenshortDir
}

// check if the file is an image
func isImage(filename string) bool {
	for _, fileType := range ACCEPTED_FILES {
		if strings.Contains(filename, fileType) {
			return true
		}
	}
	return false
}

// process an image
func extract(client *gosseract.Client, docs *[]Doc, file string) {
	client.SetImage(file)

	// get text from image
	text, err := client.Text()
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(text, "\n")

	for _, line := range lines {
		raw := regexp.MustCompile(`[^a-zA-Z0-9 ]+`).ReplaceAllString(line, "")
		lower := strings.ToLower(raw)
		trimmed := strings.TrimSpace(lower)
		*docs = append(*docs, Doc{Filename: file, Text: trimmed})
	}
}

// delete all from index
func deleteAll(db *storm.DB) {
	var docs []Doc
	err := db.All(&docs)
	if err != nil {
		log.Fatal(err)
	}

	for _, doc := range docs {
		err = db.DeleteStruct(&doc)
		if err != nil {
			log.Fatal(err)
		}
	}
}

// save to database
func saveDoc(db *storm.DB, doc *Doc) {
	err := db.Save(doc)
	if err != nil {
		log.Fatal(err)
	}
}

// open the file in preview on mac
func open(file string) error {
	cmd := exec.Command("open", file)
	return cmd.Run()
}

// index the screenshot files
func index() {
	// docs
	var docs []Doc

	// tesseract ocr client
	client := gosseract.NewClient()
	defer client.Close()

	// create database
	db, err := storm.Open("storm.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// read files in directory
	files, err := os.ReadDir(getScreenshotDir())
	if err != nil {
		log.Fatal(err)
	}

	// extract text from files
	for _, filename := range files {
		if isImage(filename.Name()) {
			extract(client, &docs, fmt.Sprintf("%s/%s", getScreenshotDir(), filename.Name()))
		}
	}

	// clear out index
	deleteAll(db)

	for _, doc := range docs {
		saveDoc(db, &doc)
	}
}

// search for matching docs
func search(term string) *[]Doc {
	var docs []Doc

	// create database
	db, err := storm.Open("storm.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// get all the documents
	err = db.All(&docs)
	if err != nil {
		log.Fatal(err)
	}

	// fuzzy match documents
	var results []Doc
	for _, doc := range docs {
		if fuzzy.Match(strings.ToLower(term), doc.Text) {
			results = append(results, doc)
		}
	}

	return &results
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "sfzf",
		Short: "search your screenshots with fuzzy search",
	}

	var indexCmd = &cobra.Command{
		Use:   "index",
		Short: "index your screenshorts with ocr",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("indexing your screenshots")
			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Start()
			index()
			s.Stop()
		},
	}

	var searchCommand = &cobra.Command{
		Use:   "search",
		Short: "search using sfzf",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 1 {
				fmt.Println("Please provide a search term.")
				return
			}
			searchTerm := args[0]
			docs := search(searchTerm)

			// menu options
			var options []string
			for _, doc := range *docs {
				options = append(options, doc.Text)
			}

			// Create a select menu
			prompt := promptui.Select{
				Label: "open an image",
				Items: options,
			}

			// Run the menu and get the selected option
			index, _, err := prompt.Run()
			if err != nil {
				log.Fatalf("Prompt failed %v\n", err)
			}

			// get the selected
			selected := (*docs)[index]

			err = open(selected.Filename)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCommand)

	rootCmd.Execute()
}
