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
)

var tiffEndianessLittle = binary.BigEndian.Uint16([]byte("II"))
var tiffEndianessBig = binary.BigEndian.Uint16([]byte("MM"))

type fileMetaData struct {
	fileName  string
	fileExt   string
	mediaTime string
}

type renameOperation struct {
	sourceName string
	targetName string
}

//
// START: MEDIA DATA EXTRACTION
//

func photoAppendDateValueOffsetsFromIFD(fileName string, file *os.File, bo binary.ByteOrder, dateTagOffsets []uint32) ([]uint32, uint32) {
	// 2-byte count of the number of directory entries (i.e., the number of fields)
	var fields uint16
	err := binary.Read(file, bo, &fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading number of IFD entries %s: %v\n", fileName, err)
		os.Exit(1)
	}

	// EXIF IFD will be needed after parsing all current IFDs:
	var exifOffset uint32

	for t := 0; t < int(fields); t++ {
		// Bytes 0-1 The Tag that identifies the field
		var fieldTag uint16
		err := binary.Read(file, bo, &fieldTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IFD tag %s: %v\n", fileName, err)
			os.Exit(1)
		}

		// Bytes 2-3 The field Type
		var fieldType uint16
		err = binary.Read(file, bo, &fieldType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IFD type %s: %v\n", fileName, err)
			os.Exit(1)
		}

		// Bytes 4-7 The number of values, Count of the indicated Type
		var fieldCount uint32
		err = binary.Read(file, bo, &fieldCount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IFD count %s: %v\n", fileName, err)
			os.Exit(1)
		}

		// Bytes 8-11 The Value Offset, the file offset (in bytes) of the Value for the field
		var fieldValueOffset uint32
		err = binary.Read(file, bo, &fieldValueOffset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IFD value offset %s: %v\n", fileName, err)
			os.Exit(1)
		}

		// 0x0132: DateTime
		// 0x9003: DateTimeOriginal
		// 0x9004: DateTimeDigitized
		if fieldTag == 0x0132 || fieldTag == 0x9003 || fieldTag == 0x9004 {
			if fieldType != 2 {
				fmt.Fprintf(os.Stderr, "expected tag has unexpected type in file %s: %d == %d\n", fileName, fieldTag, fieldType)
				os.Exit(1)
			}
			if fieldCount != 20 {
				fmt.Fprintf(os.Stderr, "expected tag has unexpected size in file %s: %d == %d\n", fileName, fieldTag, fieldCount)
				os.Exit(1)
			}
			dateTagOffsets = append(dateTagOffsets, fieldValueOffset)
		}

		// 0x8769: ExifIFDPointer
		if fieldTag == 0x8769 {
			if fieldType != 4 {
				fmt.Fprintf(os.Stderr, "EXIF pointer tag has unexpected type in file %s: %d == %d\n", fileName, fieldTag, fieldType)
				os.Exit(1)
			}
			if fieldCount != 1 {
				fmt.Fprintf(os.Stderr, "EXIF pointer tag has unexpected size in file %s: %d == %d\n", fileName, fieldTag, fieldCount)
				os.Exit(1)
			}
			exifOffset = fieldValueOffset
		}
	}

	return dateTagOffsets, exifOffset
}

// https://www.adobe.io/content/dam/udp/en/open/standards/tiff/TIFF6.pdf
func photoParseTiffEarliestDate(file *os.File, fileName string) string {
	// Bytes 0-1: The byte order used within the file. Legal values are:
	// “II” (4949.H)
	// “MM” (4D4D.H)
	var tiffEndianess uint16
	// smart thing about specification, we can supplly any endianess:
	err := binary.Read(file, binary.LittleEndian, &tiffEndianess)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file header %s: %v\n", fileName, err)
		os.Exit(1)
	}

	// In the “II” format, byte order is always from the least significant byte to the most
	// significant byte, for both 16-bit and 32-bit integers.
	// This is called little-endian byte order.
	//  In the “MM” format, byte order is always from most significant to least
	// significant, for both 16-bit and 32-bit integers.
	// This is called big-endian byte order
	var bo binary.ByteOrder
	switch tiffEndianess {
	case tiffEndianessBig:
		bo = binary.BigEndian
	case tiffEndianessLittle:
		bo = binary.LittleEndian
	default:
		fmt.Fprintf(os.Stderr, "invalid TIFF file header for file %s: %v\n", fileName, tiffEndianess)
		os.Exit(1)
	}

	// Bytes 2-3 An arbitrary but carefully chosen number (42)
	// that further identifies the file as a TIFF file.
	var tiffMagic uint16
	err = binary.Read(file, bo, &tiffMagic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading TIFF magic number %s: %v\n", fileName, err)
		os.Exit(1)
	}
	if tiffMagic != 42 {
		fmt.Fprintf(os.Stderr, "invalid TIFF magic number %s: %v\n", fileName, tiffMagic)
		os.Exit(1)
	}

	// Bytes 4-7 The offset (in bytes) of the first IFD.
	var ifdOffset uint32
	err = binary.Read(file, bo, &ifdOffset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading IFD offset %s: %v\n", fileName, err)
		os.Exit(1)
	}

	// offsets for date tags we are looking for:
	var dateTagOffsets []uint32
	// offset for EXIF IFD:
	var exifOffset uint32

	// saving previous offset to protect against recursive IFD:
	var ifdOffsetPrev = ifdOffset
	for ifdOffset != 0 {
		// seek the IFD:
		_, err := file.Seek(int64(ifdOffset), 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error seeking IFD offset %s: %v\n", fileName, err)
			os.Exit(1)
		}

		dateTagOffsets, exifOffset = photoAppendDateValueOffsetsFromIFD(fileName, file, bo, dateTagOffsets)

		// we are looking for only 3 tags:
		if len(dateTagOffsets) == 3 {
			break
		}

		err = binary.Read(file, bo, &ifdOffset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading next IFD offset: %s\n", fileName)
			os.Exit(1)
		}

		if ifdOffset == ifdOffsetPrev {
			fmt.Fprintf(os.Stderr, "recursive IFD is not supported: %s\n", fileName)
			os.Exit(1)
		}
		// if EXIF offset matches current offset then skip EXIF:
		if ifdOffset == exifOffset {
			exifOffset = 0
		}
		ifdOffsetPrev = ifdOffset
	}
	// read EXIF IFD:
	for exifOffset != 0 {
		_, err = file.Seek(int64(exifOffset), 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error seeking EXIF offset %s: %v\n", fileName, err)
			os.Exit(1)
		}
		var exifOffsetPrev = exifOffset
		dateTagOffsets, exifOffset = photoAppendDateValueOffsetsFromIFD(fileName, file, bo, dateTagOffsets)
		// protection from recursive offsets:
		if exifOffset == exifOffsetPrev {
			break
		}
	}

	if len(dateTagOffsets) == 0 {
		fmt.Fprintf(os.Stderr, "no date tags found in file: %s\n", fileName)
		os.Exit(1)
	}
	// sort to read from closest tag:
	sort.Slice(dateTagOffsets, func(i, j int) bool {
		return dateTagOffsets[i] < dateTagOffsets[j]
	})

	// 2 = ASCII 8-bit byte that contains a 7-bit ASCII code; the last byte must be NUL (binary zero).
	// tag count is 20, which means 19 chars and binary NUL,
	// we will read only 19 bytes then:
	var earliestDate string
	var buffer = make([]byte, 19)
	for _, tagOffset := range dateTagOffsets {
		_, err = file.Seek(int64(tagOffset), 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error seeking date tag value %s: %v\n", fileName, err)
			os.Exit(1)
		}
		_, err = file.Read(buffer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading date tag value %s: %v\n", fileName, err)
			os.Exit(1)
		}
		if len(earliestDate) == 0 {
			earliestDate = string(buffer)
		} else {
			str := string(buffer)
			if str < earliestDate {
				earliestDate = str
			}
		}
	}

	parsed, parseError := time.Parse("2006:01:02 15:04:05", earliestDate)
	if parseError != nil {
		// bug in Samsung S9 camera, panorama photo has different date format:
		parsed2, parseError2 := time.Parse("2006-01-02 15:04:05", earliestDate)
		if parseError != nil {
			fmt.Fprintf(os.Stderr, "error parsing exif date in file %s:\n\t%v\n\t%v\n", fileName, parseError, parseError2)
			os.Exit(1)
		}
		parsed = parsed2
	}
	return parsed.Format("20060102-150405")
}

func photoEarliestTime(fileName string) string {
	openFile, openError := os.Open(fileName)
	if openError != nil {
		fmt.Fprintf(os.Stderr, "error opening file %s: %v\n", fileName, openError)
		os.Exit(1)
	}
	defer func() {
		closeError := openFile.Close()
		if closeError != nil {
			fmt.Fprintf(os.Stderr, "error closing file %s: %v\n", fileName, closeError)
			os.Exit(1)
		}
	}()
	return photoParseTiffEarliestDate(openFile, fileName)
}

func videoReadAtLeast(file *os.File, buffer []byte) {
	_, readErr := io.ReadAtLeast(file, buffer, 8)
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "error reading media file: %v\n", readErr)
		os.Exit(1)
	}
}

func videoNextAtom(file *os.File, buffer []byte) (uint32, string) {
	videoReadAtLeast(file, buffer)
	len := binary.BigEndian.Uint32(buffer[:4])
	typeStr := string(buffer[4:])
	return len, typeStr
}

func videoEarliestTime(fileName string) string {
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
		len, typeStr := videoNextAtom(file, buffer)
		if typeStr == "moov" {
			var processedMoov uint32 = 8
			for {
				l, t := videoNextAtom(file, buffer)
				if t == "mvhd" {
					videoReadAtLeast(file, buffer)
					version := buffer[0]
					var seconds uint64
					videoReadAtLeast(file, buffer)
					if version == 1 {
						seconds = binary.BigEndian.Uint64(buffer)
					} else {
						seconds = uint64(binary.BigEndian.Uint32(buffer[:4]))
					}
					// mp4 starts from year 1904:
					offset := uint64(time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
					// adding negative offset:
					seconds += offset
					return time.Unix(int64(seconds), 0).UTC().Format("20060102-150405")
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
		return photoEarliestTime(fileName)
	}
	if isSupportedVideo(fileExt) {
		return videoEarliestTime(fileName)
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

func verifyOperations(operations []renameOperation, longestSourceName int) {
	format := fmt.Sprintf("    %%%ds    =>    %%s\n", longestSourceName)
	duplicatesMap := make(map[string]string)
	for _, operation := range operations {
		fmt.Printf(format, operation.sourceName, operation.targetName)
		// check for target name duplicates:
		if _, existsInMap := duplicatesMap[operation.targetName]; existsInMap {
			fmt.Fprintf(os.Stderr, "\ntarget file name duplicate: %s\n", operation.targetName)
			os.Exit(1)
		} else {
			duplicatesMap[operation.targetName] = operation.targetName
		}
		// check for renaming duplicates:
		if operation.sourceName != operation.targetName {
			if _, existsInDir := os.Stat(operation.targetName); existsInDir == nil {
				fmt.Fprintf(os.Stderr, "\ntarget file exists on file system: %s\n", operation.targetName)
				os.Exit(1)
			}
		}
	}
}

func main() {
	var dryRun bool
	var noPrefix bool
	flag.BoolVar(&dryRun, "d", false, "dry run")
	flag.BoolVar(&noPrefix, "p", false, "no counter prefix")
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
	var targetFormat string
	if noPrefix {
		targetFormat = "%s%s"
	} else {
		targetFormat = targetFileNameFormat(len(metaData))
	}
	for index, md := range metaData {
		var targetName string
		// different target name depending on prefix flag:
		if noPrefix {
			targetName = fmt.Sprintf(targetFormat, md.mediaTime, md.fileExt)
		} else {
			targetName = fmt.Sprintf(targetFormat, index+1, md.mediaTime, md.fileExt)
		}
		operations = append(operations, renameOperation{md.fileName, targetName})
		// choosing longest source file name for next operation:
		sourceNameLength := len(md.fileName)
		if sourceNameLength > longestSourceName {
			longestSourceName = sourceNameLength
		}
	}
	fmt.Println(" done.")
	fmt.Println("verifying:")
	verifyOperations(operations, longestSourceName)
	fmt.Println("done.\n")
	totalOperations := len(operations)
	for index, f := range operations {
		fmt.Printf("\rrenaming files: %d/%d...", index+1, totalOperations)
		if !dryRun {
			renameError := os.Rename(f.sourceName, f.targetName)
			if renameError != nil {
				fmt.Fprintf(os.Stderr, "\nerror renaming file %s: %v\n", f.sourceName, renameError)
				os.Exit(1)
			}
			chmodError := os.Chmod(f.targetName, 0444)
			if chmodError != nil {
				fmt.Fprintf(os.Stderr, "\nerror chmoding file %s: %v\n", f.sourceName, chmodError)
				os.Exit(1)
			}
		}
	}
	fmt.Println("\n\nfinished.")
}
