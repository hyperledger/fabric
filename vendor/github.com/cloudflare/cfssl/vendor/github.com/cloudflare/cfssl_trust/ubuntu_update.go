// USAGE: go run ubuntu_update.go
// This script generates ubuntu's trust store in a file ubuntu.pem
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Change updateURL and packageName as necessary to ubuntu's ca-certificates update
var updateURL = "http://archive.ubuntu.com/ubuntu/pool/main/c/ca-certificates/"
var packageName = "ca-certificates_20150426ubuntu1.tar.xz"

// "tar xvf packageName" will create a directory without the ".tar.xz" extension
// and will also replace all underscores "_" with dashes "-"
var dirName = strings.Replace(strings.TrimSuffix(packageName, ".tar.xz"), "_", "-", -1)

func main() {
	// Download the package to local directory
	output, _ := os.Create(packageName)
	defer output.Close()

	res, _ := http.Get(updateURL + packageName)
	defer res.Body.Close()

	n, _ := io.Copy(output, res.Body)
	fmt.Println(n, "bytes downloaded.")

	// Untar the package downloaded
	exec.Command("tar", "xvf", packageName).Run()

	// Run make to load the trust store
	exec.Command("make", "-C", dirName).Run()

	// Create the ubuntu trust store file
	newfile, _ := os.Create("ubuntu.pem")

	// Open up the trust store directory
	files, _ := ioutil.ReadDir(dirName + "/mozilla")
	count := 0

	// Loop through every file
	for _, f := range files {
		file, _ := os.Open(dirName + "/mozilla/" + f.Name())
		defer file.Close()
		if strings.Contains(f.Name(), ".crt") {
			scanner := bufio.NewScanner(file)
			// Loop through *.crt, append lines to file
			for scanner.Scan() {
				newfile.Write([]byte(scanner.Text() + "\n"))
			}
			file.Close()
			fmt.Println(f.Name())
			count++
		}
	}
	fmt.Println(count)
	newfile.Close()
	os.Remove(packageName)
	os.RemoveAll(dirName)
}
