// This script takes a file containing multiple certificates and a directory,
// and creates all the certificates in their own files via a naming convention
// using info from cfssl certinfo. These created files are put in the directory
// provided.

// Usage: go run format.go -f int-bundle.crt -d intermediate_ca
// This will take int-bundle.crt, a file full of certificates, and create all
// the individual certificates from this file and put them in the
// the intermediate_ca directory (in the current working directory).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cloudflare/cfssl/helpers"
)

// filename is the file full of certificates provided in CL
var filename = new(string)

// directory is the destination of our created certificates
var directory = new(string)

// map holds all of the filenames and a count to account for duplicates
var filenameCount = make(map[string]int)

// Parse arguments from the command line.
// To run the script: "go run format.go -f filename -d directory"
func parseArgs() int {
	filename = flag.String("f", "", "file of certificates we want to separate")
	directory = flag.String("d", "", "directory to put the separated"+
		" certificates into")
	flag.Parse()

	// Return error if filename or directory are not specified
	if *filename == "" {
		fmt.Println("Filename not specified!")
		return 1
	}
	if *directory == "" {
		fmt.Println("Directory not specified!")
		return 1
	}
	return 0
}

// Applies the naming convention of "commonname_issuedate_sigalg.crt" to all
// files by renaming them using helpers.ParseCertificatePEM
func applyNamingConv(file []byte, filename string) error {
	// Get the certificate info from helpers.ParseCertificatePEM
	response, err := helpers.ParseCertificatePEM(file)
	if err != nil {
		fmt.Println(err)
		return err
	}
	// Generate the name of the file
	name := ""
	// If CommonName field is empty, then use the Organization name instead
	if len(response.Subject.CommonName) == 0 {
		name = strings.Replace(response.Subject.Organization[0], " ", "", -1)
		name = strings.Replace(name, "/", "-", -1)
	} else {
		name = strings.Replace(response.Subject.CommonName, " ", "", -1)
		name = strings.Replace(name, "/", "-", -1)
	}
	dateIssued := fmt.Sprintf("%d-%d-%d", response.NotBefore.Year(),
		int(response.NotBefore.Month()),
		response.NotBefore.Day())
	name += "_" + dateIssued + "_" + helpers.SignatureString(response.SignatureAlgorithm)

	// If there is already a certificate of the same name, then add a count
	// at the end AKA, "commonname_issuedate_sigalg_2.crt" if there is
	// already a "commonname_issuedate_sigalg.crt", etc...
	filenameCount[name]++
	if filenameCount[name] > 1 {
		name += "_" + strconv.Itoa(filenameCount[name])
	}

	// Finally, rename the file
	err = os.Rename(*directory+"/"+filename, *directory+"/"+name+".crt")
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// First, parse the CL arguments
	if parseArgs() == 1 {
		return
	}

	count := 0 // # of files to be created
	strFile := ""

	// Open up the file of certificates to read
	store, _ := os.Open(*filename)
	defer store.Close()
	scanner := bufio.NewScanner(store)
	// First default file "1.crt"; this will be overridden in the for loop
	newfile, _ := os.Create(*directory + "/" + strconv.Itoa(1) + ".crt")

	// Read through the file
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "-----BEGIN CERTIFICATE-----") {
			count++
			strFile = ""
			// Create the certificate file in directory
			newfile, _ = os.Create(*directory + "/" + strconv.Itoa(count) +
				".crt")
		}
		// Copy over each line into the new file
		newfile.Write([]byte(scanner.Text() + "\n"))
		strFile += scanner.Text() + "\n"

		// Wrap up file to rename when end is encountered
		if strings.Contains(scanner.Text(), "-----END CERTIFICATE-----") {
			// Close the file and rename it
			newfile.Close()
			err := applyNamingConv([]byte(strFile), strconv.Itoa(count)+".crt")
			if err != nil {
				// Printing error will show which file(s) did not successfully
				// get renamed
				fmt.Println(err)
			}
		}
	}
	// Prints the count of how many certificates created
	fmt.Println(count)
	store.Close()
	newfile.Close()
}
