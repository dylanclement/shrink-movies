package main

import (
	"flag"
	"fmt"
	"io"
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

// CopyFile Helper function to copy a file
func CopyFile(src, dst string) error {
	// open input file
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// create dest file
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// copy contents from source to destination
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// Swaps 2 files
func swapFiles(inFile, outFile string) string {
	// create new temp dir
	swapDir, err := ioutil.TempDir("", "swap")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(swapDir) // clean up

	// swap files around, first move source to temp, then move dest to source
	if err := CopyFile(inFile, filepath.Join(swapDir, filepath.Base(inFile))); err != nil {
		log.Error(err)
	}
	os.Remove(inFile)

	destFileName := filepath.Join(filepath.Dir(inFile), filepath.Base(outFile))
	if err := CopyFile(outFile, destFileName); err != nil {
		log.Error(err)
	}
	os.Remove(outFile)

	return destFileName
}

// Processes a single photo file, copying it to the output dir and creating thumbnails etc. in S3
func processFile(sourceFile, outDir, tmpDir string) error {
	modTime := getFileModTime(sourceFile)

	// Get an output file name, make all files mp4  and make sure we can support multiple files in the same dir
	destFile := filepath.Join(tmpDir, modTime.Format("20060102_150405")+".mp4")
	for i := 1; ; i++ {
		if _, err := os.Stat(destFile); os.IsNotExist(err) {
			break
		}
		destFile = filepath.Join(tmpDir, fmt.Sprintf(modTime.Format("20060102_150405")+"_%04d.mp4", i))
	}

	// Run ffmpeg on the input file and save to output dir
	cmd := exec.Command("ffmpeg", "-i", sourceFile, "-c:v", "libx264", "-preset", "slow", "-crf", "28", "-movflags", "+faststart", "-c:a", "copy", destFile)
	if err := cmd.Run(); err != nil {
		log.Error("Could not run ffmpeg on file: ", sourceFile, err)
		return err
	}

	// Check what the ratio input/output is
	inSize := getFileSize(sourceFile)
	outSize := getFileSize(destFile)
	ratio := float64(outSize) / float64(inSize)
	if ratio < 0.93 {
		newDestFile := swapFiles(sourceFile, destFile)
		// Make sure new file has the same mod time as original file
		if err := os.Chtimes(newDestFile, modTime, modTime); err != nil {
			log.Error(err)
		}
	}

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

	// Process each file in directory
	for _, fileName := range fileList {
		processFile(fileName, outDirName, tmpDir)
	}
}

func main() {
	inDirNamePtr := flag.String("i", "", "input directory")
	outDirNamePtr := flag.String("o", "", "output directory")

	flag.Parse()
	if len(*inDirNamePtr) == 0 {
		log.Fatal("Error, need to define an input directory.")
	}

	// Create temp dir and remember to clean up
	tmpDir, _ := ioutil.TempDir("", "shrink-file")
	defer os.RemoveAll(tmpDir) // clean up

	process(*inDirNamePtr, *outDirNamePtr, tmpDir)
	log.Info("Done processing: ", *inDirNamePtr)
}
