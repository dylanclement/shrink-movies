package main

import (
	"flag"
	"io/ioutil"
	filepath "path/filepath"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
)

// Processes a single photo file, copying it to the output dir and creating thumbnails etc. in S3
func processFile(sourceFile, outDir string) error {
	log.Info("Processing File: ", sourceFile)
	return nil
}

// IsMovie returns true is the file is a movie
func IsMovie(fileName string) bool {
	fileExt := strings.ToLower(filepath.Ext(fileName))
	return fileExt == ".mpg" || fileExt == ".mpeg" || fileExt == ".avi" || fileExt == ".mp4" || fileExt == ".3gp" || fileExt == ".mov"
}

// Gets all files in directory
func addFilesToList(inDirName string, fileList *[]string) {
	files, err := ioutil.ReadDir(inDirName)
	if err != nil {
		log.Fatal(err.Error())
	}

	for _, f := range files {
		if f.IsDir() {
			dirName := f.Name()
			if dirName[0] == '.' {
				continue
			}
			addFilesToList(filepath.Join(inDirName, dirName), fileList)
		} else {
			if IsMovie(f.Name()) {
				fileName := filepath.Join(inDirName, f.Name())
				*fileList = append(*fileList, fileName)
			}
		}
	}
}

// Loops through all files in a dir and processes them all
func process(inDirName, outDirName string) {
	// Get all files in directory
	var fileList []string
	addFilesToList(inDirName, &fileList)

	// Since we are using go routines to process the files, create channels and sync waits
	sem := make(chan int, 4) // Have 8 running concurrently
	var wg sync.WaitGroup

	// Organise photos by moving to target folder or uploading it to S3
	for _, fileName := range fileList {

		// Remember to increment the waitgroup by 1 BEFORE starting the goroutine
		wg.Add(1)
		go func(fileNameInner string) {
			sem <- 1 // Wait for active queue to drain.
			err := processFile(fileNameInner, outDirName)
			if err != nil {
				log.Fatal(err.Error())
			}

			wg.Done()
			<-sem // Done; enable next request to run.
		}(fileName)
	}
	wg.Wait() // Wait for all goroutines to finish
}

func main() {
	inDirNamePtr := flag.String("i", "", "input directory")
	outDirNamePtr := flag.String("o", "", "output directory")

	flag.Parse()
	if len(*inDirNamePtr) == 0 {
		log.Fatal("Error, need to define an input directory.")
	}

	process(*inDirNamePtr, *outDirNamePtr)
	log.Info("Done processing: ", *inDirNamePtr)
}
