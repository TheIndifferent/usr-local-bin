package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {

	flag.Parse()
	keyLength := 16
	if flag.NArg() > 0 {
		i, err := strconv.Atoi(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error parsing the argument")
		} else if i > 0 {
			keyLength = i
		}
	}

	fmt.Printf("generating key of %d bytes:\n", keyLength)
	b := make([]byte, keyLength)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error in random generator")
		os.Exit(1)
	}
	b16 := hex.EncodeToString(b)
	s256 := sha256.Sum256(b)
	s16 := hex.EncodeToString(s256[:])
	fmt.Print("key:    ")
	fmt.Println(b16)
	fmt.Print("sha256: ")
	fmt.Println(s16)
}
