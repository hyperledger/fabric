package main

import (
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"time"

	"encoding/base64"
	"github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/scanner"
	"github.com/mreiferson/go-httpclient"
)

const (
	// A regex which cannot match any input
	MatchesNothingRegex = "a^"
)

var logUri = flag.String("log_uri", "http://ct.googleapis.com/aviator", "CT log base URI")
var matchSubjectRegex = flag.String("match_subject_regex", ".*", "Regex to match CN/SAN")
var matchIssuerRegex = flag.String("match_issuer_regex", "", "Regex to match in issuer CN")
var precertsOnly = flag.Bool("precerts_only", false, "Only match precerts")
var serialNumber = flag.String("serial_number", "", "Serial number of certificate of interest")
var batchSize = flag.Int("batch_size", 1000, "Max number of entries to request at per call to get-entries")
var numWorkers = flag.Int("num_workers", 2, "Number of concurrent matchers")
var parallelFetch = flag.Int("parallel_fetch", 2, "Number of concurrent GetEntries fetches")
var startIndex = flag.Int64("start_index", 0, "Log index to start scanning at")
var quiet = flag.Bool("quiet", false, "Don't print out extra logging messages, only matches.")
var printChains = flag.Bool("print_chains", false, "If true prints the whole chain rather than a summary")

// Prints out a short bit of info about |cert|, found at |index| in the
// specified log
func logCertInfo(entry *ct.LogEntry) {
	log.Printf("Interesting cert at index %d: CN: '%s'", entry.Index, entry.X509Cert.Subject.CommonName)
}

// Prints out a short bit of info about |precert|, found at |index| in the
// specified log
func logPrecertInfo(entry *ct.LogEntry) {
	log.Printf("Interesting precert at index %d: CN: '%s' Issuer: %s", entry.Index,
		entry.Precert.TBSCertificate.Subject.CommonName, entry.Precert.TBSCertificate.Issuer.CommonName)
}

func chainToString(certs []ct.ASN1Cert) string {
	var output []byte

	for _, cert := range certs {
		output = append(output, cert...)
	}

	return base64.StdEncoding.EncodeToString(output)
}

func logFullChain(entry *ct.LogEntry) {
	log.Printf("Index %d: Chain: %s", entry.Index, chainToString(entry.Chain))
}

func createRegexes(regexValue string) (*regexp.Regexp, *regexp.Regexp) {
	// Make a regex matcher
	var certRegex *regexp.Regexp
	precertRegex := regexp.MustCompile(regexValue)
	switch *precertsOnly {
	case true:
		certRegex = regexp.MustCompile(MatchesNothingRegex)
	case false:
		certRegex = precertRegex
	}

	return certRegex, precertRegex
}

func createMatcherFromFlags() (scanner.Matcher, error) {
	if *matchIssuerRegex != "" {
		certRegex, precertRegex := createRegexes(*matchIssuerRegex)
		return scanner.MatchIssuerRegex{
			CertificateIssuerRegex:    certRegex,
			PrecertificateIssuerRegex: precertRegex}, nil
	}
	if *serialNumber != "" {
		log.Printf("Using SerialNumber matcher on %s", *serialNumber)
		var sn big.Int
		_, success := sn.SetString(*serialNumber, 0)
		if !success {
			return nil, fmt.Errorf("Invalid serialNumber %s", *serialNumber)
		}
		return scanner.MatchSerialNumber{SerialNumber: sn}, nil
	} else {
		certRegex, precertRegex := createRegexes(*matchSubjectRegex)
		return scanner.MatchSubjectRegex{
			CertificateSubjectRegex:    certRegex,
			PrecertificateSubjectRegex: precertRegex}, nil
	}
}

func main() {
	flag.Parse()
	logClient := client.New(*logUri, &http.Client{
		Transport: &httpclient.Transport{
			ConnectTimeout:        10 * time.Second,
			RequestTimeout:        30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   10,
			DisableKeepAlives:     false,
			},
		})
	matcher, err := createMatcherFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	opts := scanner.ScannerOptions{
		Matcher:       matcher,
		BatchSize:     *batchSize,
		NumWorkers:    *numWorkers,
		ParallelFetch: *parallelFetch,
		StartIndex:    *startIndex,
		Quiet:         *quiet,
	}
	scanner := scanner.NewScanner(logClient, opts)

	if *printChains {
		scanner.Scan(logFullChain, logFullChain)
	} else {
		scanner.Scan(logCertInfo, logPrecertInfo)
	}
}
