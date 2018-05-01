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

//
// START: MEDIA DATA EXTRACTION
//

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

func mediaTimestamp(fileName string, fileExt string) string {
	if isSupportedPhoto(fileExt) {
		return earliestExifTime(fileName)
	}
	if isSupportedVideo(fileExt) {
		return earliestMediaTime(fileName)
	}
	return ""
}

//
// END: MEDIA DATA EXTRACTION
//

func isSupportedPhoto(ext string) bool {
	return ext == ".nef" || ext == ".dng" || ext == ".jpg"
}

func isSupportedVideo(ext string) bool {
	return ext == ".mp4"
}

func listFiles() (int, []os.FileInfo) {
	dirContent, dirContentErr := ioutil.ReadDir(".")
	if dirContentErr != nil {
		fmt.Fprintf(os.Stderr, "error listing dir: %v\n", dirContentErr)
		os.Exit(1)
	}
	return len(dirContent), dirContent
}

func targetFileNameFormat(numberOfFiles int) string {
	if numberOfFiles < 10 {
		return "%d-%s%s"
	}
	if numberOfFiles < 100 {
		return "%02d-%s%s"
	}
	if numberOfFiles < 1000 {
		return "%03d-%s%s"
	}
	if numberOfFiles < 10000 {
		return "%04d-%s%s"
	}
	if numberOfFiles < 100000 {
		return "%05d-%s%s"
	}
	fmt.Fprintf(os.Stderr, "too many files: %d\n", numberOfFiles)
	os.Exit(1)
	return ""
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

type fileMetaData struct {
	fileName  string
	fileExt   string
	mediaTime string
}

type renameOperation struct {
	sourceName string
	targetName string
}

func main() {
	var dryRun bool
	flag.BoolVar(&dryRun, "n", false, "dry run")
	flag.Parse()
	if dryRun {
		fmt.Println("Dry run:")
	}

	var metaData []fileMetaData
	numberOfFiles, files := listFiles()
	for index, file := range files {
		fmt.Printf("\rprocessing files: %d/%d...", index+1, numberOfFiles)
		// skipping dirs:
		if file.IsDir() {
			continue
		}
		// extracting media timestamps:
		ext := strings.ToLower(filepath.Ext(file.Name()))
		mediaTime := mediaTimestamp(file.Name(), ext)
		// skipping unsupported formats:
		if mediaTime == "" {
			continue
		}
		// creating everything needed for sorting:
		metaData = append(metaData, fileMetaData{file.Name(), ext, mediaTime})
	}
	fmt.Println(" done.")
	fmt.Printf("sorting %d supported files...", len(metaData))
	sort.Slice(metaData, func(i, j int) bool {
		a := metaData[i]
		b := metaData[j]
		if a.mediaTime == b.mediaTime {
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
		return a.mediaTime < b.mediaTime
	})
	fmt.Println(" done.")
	fmt.Print("preparing rename operations...")
	longestSourceName := 0
	var operations []renameOperation
	targetFormat := targetFileNameFormat(len(metaData))
	for index, md := range metaData {
		targetName := fmt.Sprintf(targetFormat, index+1, md.mediaTime, md.fileExt)
		operations = append(operations, renameOperation{md.fileName, targetName})
		// choosing longest source file name for next operation:
		sourceNameLength := len(md.fileName)
		if sourceNameLength > longestSourceName {
			longestSourceName = sourceNameLength
		}
	}
	fmt.Println(" done.")
	fmt.Println("renaming:")
	format := fmt.Sprintf("    %%%ds    =>    %%s\n", longestSourceName)
	for _, f := range operations {
		fmt.Printf(format, f.sourceName, f.targetName)
		if !dryRun {
			os.Rename(f.sourceName, f.targetName)
			os.Chmod(f.targetName, 0444)
		}
	}
	fmt.Println("finished.")
}
