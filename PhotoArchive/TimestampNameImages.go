package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xor-gate/goexif2/exif"
)

type supportedFile struct {
	fileName string
	fileExt  string
}

type sortingEntry struct {
	fileName string
	fileExt  string
	exifTime string
}

type renameOperation struct {
	sourceName string
	targetName string
}

func listFiles() []string {
	dirContent, dirContentErr := ioutil.ReadDir(".")
	if dirContentErr != nil {
		fmt.Fprintf(os.Stderr, "error listing dir: %v\n", dirContentErr)
		os.Exit(1)
	}

	var files []string
	for _, f := range dirContent {
		if !f.IsDir() {
			files = append(files, f.Name())
		}
	}
	return files
}

func isSupportedPhoto(ext string) bool {
	return ext == ".nef" || ext == ".dng" || ext == ".jpg"
}

func isSupportedVideo(ext string) bool {
	return ext == ".mp4"
}

func supportedFiles(files []string) []supportedFile {
	var list []supportedFile
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if isSupportedPhoto(ext) || isSupportedVideo(ext) {
			list = append(list, supportedFile{f, ext})
		}
	}
	return list
}

func getExifStringValue(exifData *exif.Exif, fieldName exif.FieldName) string {
	tag, tagError := exifData.Get(fieldName)
	if tagError != nil {
		fmt.Fprintf(os.Stderr, "error reading exif tag: %v\n", tagError)
		os.Exit(1)
	}
	str, strErr := tag.StringVal()
	if strErr != nil {
		fmt.Fprintf(os.Stderr, "error converting tag to string: %v\n", strErr)
		os.Exit(1)
	}
	return str
}

func earliestExifTime(file string) string {
	openFile, openError := os.Open(file)
	if openError != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %v\n", openError)
		os.Exit(1)
	}
	defer func() {
		closeError := openFile.Close()
		if closeError != nil {
			fmt.Fprintf(os.Stderr, "error closing file: %v\n", closeError)
			os.Exit(1)
		}
	}()
	exifData, exifError := exif.Decode(openFile)
	if exifError != nil {
		fmt.Fprintf(os.Stderr, "error reading exif: %v\n", exifError)
		os.Exit(1)
	}
	exifDates := []string{
		getExifStringValue(exifData, exif.DateTime),
		getExifStringValue(exifData, exif.DateTimeDigitized),
		getExifStringValue(exifData, exif.DateTimeOriginal)}
	sort.Strings(exifDates)
	earliest := exifDates[0]
	parsed, parseError := time.Parse("2006:01:02 15:04:05", earliest)
	if parseError != nil {
		fmt.Fprintf(os.Stderr, "error parsing exif date: %v\n", parseError)
		os.Exit(1)
	}
	return parsed.Format("20060102-150405")
}

func readAtLeast(file *os.File, buffer []byte) {
	_, readErr := io.ReadAtLeast(file, buffer, 8)
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "error reading media file: %v\n", readErr)
		os.Exit(1)
	}
}

func nextAtom(file *os.File, buffer []byte) (uint32, string) {
	readAtLeast(file, buffer)
	len := binary.BigEndian.Uint32(buffer[:4])
	typeStr := string(buffer[4:])
	return len, typeStr
}

func earliestMediaTime(fileName string) string {
	file, openError := os.Open(fileName)
	if openError != nil {
		fmt.Fprintf(os.Stderr, "error opening media file: %v\n", openError)
		os.Exit(1)
	}
	defer func() {
		closeError := file.Close()
		if closeError != nil {
			fmt.Fprintf(os.Stderr, "error closing file: %v\n", closeError)
			os.Exit(1)
		}
	}()
	var buffer []byte
	buffer = make([]byte, 8)
	var processed uint32
	fileStat, statError := file.Stat()
	if statError != nil {
		fmt.Fprintf(os.Stderr, "error stating media file: %v\n", statError)
		os.Exit(1)
	}
	total := uint32(fileStat.Size())
	for {
		len, typeStr := nextAtom(file, buffer)
		if typeStr == "moov" {
			var processedMoov uint32 = 8
			for {
				l, t := nextAtom(file, buffer)
				if t == "mvhd" {
					readAtLeast(file, buffer)
					version := buffer[0]
					var seconds uint64
					readAtLeast(file, buffer)
					if version == 1 {
						seconds = binary.BigEndian.Uint64(buffer)
					} else {
						seconds = uint64(binary.BigEndian.Uint32(buffer[:4]))
					}
					// mp4 starts from year 1904:
					offset := uint64(time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
					// adding negative offset:
					seconds += offset
					return time.Unix(int64(seconds), 0).Format("20060102-150405")
				} else {
					processedMoov += l
					processed += l
				}
				if processedMoov >= len {
					break
				}
				file.Seek(int64(processed), 0)
			}
		} else {
			processed += len
		}
		if processed >= total {
			break
		}
		file.Seek(int64(processed), 0)
	}
	fmt.Fprintf(os.Stderr, "movie header not found in file: %s\n", fileName)
	os.Exit(1)
	return ""
}

func sortedFiles(files []supportedFile) []sortingEntry {
	var sorting []sortingEntry
	for _, f := range files {
		if isSupportedPhoto(f.fileExt) {
			exifTime := earliestExifTime(f.fileName)
			entry := sortingEntry{f.fileName, f.fileExt, exifTime}
			sorting = append(sorting, entry)
			continue
		}
		if isSupportedVideo(f.fileExt) {
			mediaTime := earliestMediaTime(f.fileName)
			entry := sortingEntry{f.fileName, f.fileExt, mediaTime}
			sorting = append(sorting, entry)
		}
	}
	sort.Slice(sorting, func(i, j int) bool {
		a := sorting[i]
		b := sorting[j]
		if a.exifTime == b.exifTime {
			if a.fileName == b.fileName {
				fmt.Fprintf(os.Stderr, "file encountered twice: %s\n", a.fileName)
				os.Exit(1)
			}
			// workaround for Android way of dealing with same-second shots:
			// 20180430_184327.jpg
			// 20180430_184327(0).jpg
			aLen := len(a.fileName)
			bLen := len(b.fileName)
			if aLen == bLen {
				return a.fileName < b.fileName
			}
			return aLen < bLen
		}
		return a.exifTime < b.exifTime
	})
	return sorting
}

func renameOperations(files []sortingEntry) []renameOperation {
	var indexFormat string
	sortedLength := len(files)
	if sortedLength < 10 {
		indexFormat = "%d"
	} else if sortedLength < 100 {
		indexFormat = "%02d"
	} else if sortedLength < 1000 {
		indexFormat = "%03d"
	} else if sortedLength < 10000 {
		indexFormat = "%04d"
	} else if sortedLength < 100000 {
		indexFormat = "%05d"
	} else {
		fmt.Fprintf(os.Stderr, "too many files: %d\n", sortedLength)
		os.Exit(1)
	}

	var operations []renameOperation
	for index, s := range files {
		strIndex := fmt.Sprintf(indexFormat, index+1)
		operation := renameOperation{s.fileName, fmt.Sprintf("%s-%s%s", strIndex, s.exifTime, s.fileExt)}
		operations = append(operations, operation)
	}
	return operations
}

func longestSourceFileName(operations []renameOperation) int {
	longest := 0
	for _, o := range operations {
		l := len(o.sourceName)
		if l > longest {
			longest = l
		}
	}
	return longest
}

func main() {

	var dryRun bool
	flag.BoolVar(&dryRun, "n", false, "dry run")
	flag.Parse()
	if dryRun {
		fmt.Println("Dry run:")
	}

	files := listFiles()
	supported := supportedFiles(files)
	sorted := sortedFiles(supported)
	renames := renameOperations(sorted)

	longestSource := longestSourceFileName(renames)
	format := fmt.Sprintf("%%%ds    =>    %%s\n", longestSource)

	for _, f := range renames {
		fmt.Printf(format, f.sourceName, f.targetName)
		if !dryRun {
			os.Rename(f.sourceName, f.targetName)
			os.Chmod(f.targetName, 0444)
		}
	}
}
