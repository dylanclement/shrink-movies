package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	filepath "path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

// GetFileModTime Helper to get file modification time, useful as a fallback if file is not a jpg.
func getFileModTime(fileName string) time.Time {
	var containsDateRegExp = regexp.MustCompile(`^(\d{8})_.*`)
	matches := containsDateRegExp.FindStringSubmatch(fileName)
	// if filename is eg. 20160513_181656.mp4 get the date from the filename instead
	if len(matches) > 0 {
		// useful if we re-encode a badly encoded camera movie, then we don't want to use the modified date
		dateStr := matches[1]
		date, _ := time.Parse("20060102", dateStr)
		return date
	}

	// else fetch the files last modification timne
	stat, err := os.Stat(fileName)
	if err != nil {
		log.Error("Unable to get ModTime for file: ", fileName)
		return time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return stat.ModTime()
}

// Gets the size of a file in bytes
func getFileSize(fileName string) int64 {
	file, err := os.Open(fileName)
	if err != nil {
		log.Error(err)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	return fileInfo.Size()
}

// Processes a single photo file, copying it to the output dir and creating thumbnails etc. in S3
func processFile(sourceFile, outDir, tmpDir string) error {
	modTime := getFileModTime(sourceFile)

	// Get an output file name, make all files mp4  and make sure we can support multiple files in the same dir
	var destFile string
	for i := 1; ; i++ {
		destFile = filepath.Join(tmpDir, fmt.Sprintf(modTime.Format("20060102")+"_%04d.mp4", i))
		if _, err := os.Stat(destFile); os.IsNotExist(err) {
			break
		}
	}

	// Run ffmpeg on the input file and save to output dir
	cmd := exec.Command("ffmpeg", "-i", sourceFile, "-c:v", "libx264", "-preset", "slow", "-crf", "28", "-movflags", "+faststart", "-c:a", "copy", destFile)
	if err := cmd.Run(); err != nil {
		log.Error("Could not run ffmpeg on file: ", sourceFile, err)
		return err
	}

	// Make sure new file has the same mod time as original file
	if err := os.Chtimes(destFile, modTime, modTime); err != nil {
		log.Error(err)
	}

	// Check what the ratio input/output is
	inSize := getFileSize(sourceFile)
	outSize := getFileSize(destFile)
	ratio := float64(outSize) / float64(inSize)
	log.Info("Processed File: ", sourceFile, " ratio: ", ratio)
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
func process(inDirName, outDirName, tmpDir string) {
	// Get all files in directory
	var fileList []string
	addFilesToList(inDirName, &fileList)

	// Since we are using go routines to process the files, create channels and sync waits
	//sem := make(chan int, 4) // Have 8 running concurrently
	//var wg sync.WaitGroup

	// Organise photos by moving to target folder or uploading it to S3
	for _, fileName := range fileList {

		// Remember to increment the waitgroup by 1 BEFORE starting the goroutine
		/*wg.Add(1)
		go func(fileNameInner string) {
			sem <- 1 // Wait for active queue to drain.
			err := processFile(fileNameInner, outDirName, tmpDir)
			if err != nil {
				log.Fatal(err.Error())
			}

			wg.Done()
			<-sem // Done; enable next request to run.
		}(fileName)
		*/
		processFile(fileName, outDirName, tmpDir)
	}
	//wg.Wait() // Wait for all goroutines to finish
}

func main() {
	inDirNamePtr := flag.String("i", "", "input directory")
	outDirNamePtr := flag.String("o", "", "output directory")

	flag.Parse()
	if len(*inDirNamePtr) == 0 {
		log.Fatal("Error, need to define an input directory.")
	}

	// Create temp dir and remember to clean up
	//tmpDir, _ := ioutil.TempDir("", "shrink-file")
	//defer os.RemoveAll(tmpDir) // clean up
	tmpDir := "c:\\temp\\test\\output"

	process(*inDirNamePtr, *outDirNamePtr, tmpDir)
	log.Info("Done processing: ", *inDirNamePtr)
}
